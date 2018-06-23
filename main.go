//go:generate gorunpkg github.com/vektah/gqlgen -schema schema.graphql -models graph/models_generated.go -out graph/resolvers_generated.go

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/lectio/harvester"
	"github.com/lectio/lectiod/graph"
	"github.com/lectio/lectiod/service"
	"github.com/vektah/gqlgen/handler"
)

func main() {
	observatory := harvester.MakeObservatoryFromEnv()
	defer observatory.Close()

	storage := service.NewFileStorage("./tmp/diskv_data")
	service := service.NewService(observatory, storage)
	http.Handle("/", handler.Playground("Lectio", "/graphql"))
	http.Handle("/graphql", handler.GraphQL(graph.MakeExecutableSchema(service)))

	fmt.Println("Listening on :8080/graphql, saving to " + service.DefaultConfiguration().Settings().Storage.Filesys.BasePath)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
