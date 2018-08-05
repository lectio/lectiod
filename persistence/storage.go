package persistence

import (
	"fmt"

	"github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	"github.com/ipfs/go-ds-flatfs"
	"github.com/lectio/lectiod/models"
	opentracing "github.com/opentracing/opentracing-go"
	opentrext "github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
	observe "github.com/shah/observe-go"
)

type Datastore struct {
	config      *models.StorageSettings
	store       datastore.Datastore
	storeError  error
	observatory observe.Observatory
}

// NewDatastore constructs a Datastore
func NewDatastore(observatory observe.Observatory, config *models.StorageSettings, parent opentracing.Span) *Datastore {
	span := observatory.StartChildTrace("persistence.NewDatastore", parent)
	defer span.Finish()

	result := new(Datastore)
	result.config = config
	result.observatory = observatory

	span.LogFields(log.String("config.Type", string(models.StorageTypeFileSystem)))
	if config.Type == models.StorageTypeFileSystem {
		span.LogFields(log.String("config.Filesys.BasePath", string(config.Filesys.BasePath)))
		files, err := flatfs.CreateOrOpen(string(config.Filesys.BasePath), flatfs.IPFS_DEF_SHARD, true)
		if err == nil {
			result.store = files
		} else {
			error := fmt.Errorf("Unable to create flatfs in '%s': %v, creating in memory store", config.Filesys.BasePath, err)
			opentrext.Error.Set(span, true)
			span.LogFields(log.Error(error))
			result.storeError = err
			result.store = datastore.NewLogDatastore(datastore.NewMapDatastore(), "ErrorStore")
		}
	} else {
		error := fmt.Errorf("Unkown storage type '%s', creating in memory store", config.Type)
		opentrext.Error.Set(span, true)
		span.LogFields(log.Error(error))
		result.storeError = error
		result.store = datastore.NewLogDatastore(datastore.NewMapDatastore(), "ErrorStore")
	}

	return result
}

// IsValid returns true if there were no errors in constructing the datastore.
// If there was an error, an in-memory datastore is created so there's no panic.
func (d *Datastore) IsValid() bool {
	return d.storeError == nil
}

// GetError returns the storage error code if IsValid is false.
func (d *Datastore) GetError() error {
	return d.storeError
}

// Put implements Datastore.Put
func (d *Datastore) Put(key datastore.Key, value interface{}) (err error) {
	return d.store.Put(key, value)
}

// Get implements Datastore.Get
func (d *Datastore) Get(key datastore.Key) (value interface{}, err error) {
	return d.store.Get(key)
}

// Has implements Datastore.Has
func (d *Datastore) Has(key datastore.Key) (exists bool, err error) {
	return d.store.Has(key)
}

// Delete implements Datastore.Delete
func (d *Datastore) Delete(key datastore.Key) (err error) {
	return d.store.Delete(key)
}

// Query implements Datastore.Query
func (d *Datastore) Query(q dsq.Query) (dsq.Results, error) {
	return d.store.Query(q)
}

func (d *Datastore) Batch() (datastore.Batch, error) {
	return datastore.NewBasicBatch(d), nil
}

func (d *Datastore) Close() error {
	return nil
}
