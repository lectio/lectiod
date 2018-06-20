//go:generate gorunpkg github.com/vektah/gqlgen -schema schema.graphql -models graph/models_generated.go -out graph/resolvers_generated.go

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/lectio/lectiod/graph"
	"github.com/lectio/lectiod/service"
	"github.com/vektah/gqlgen/handler"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("can't initialize zap logger: %v", err)
	}
	defer logger.Sync()

	storage := service.NewFileStorage("./tmp/diskv_data")
	service := service.NewService(logger, storage)
	http.Handle("/", handler.Playground("Lectio", "/graphql"))
	http.Handle("/graphql", handler.GraphQL(graph.MakeExecutableSchema(service)))

	fmt.Println("Listening on :8080/graphql, saving to " + service.Configuration().Storage.Filesys.BasePath)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
