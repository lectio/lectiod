package main

import (
	"fmt"
	"log"

	"github.com/lectio/lectiod/server"
	observe "github.com/shah/observe-go"
)

func configPathProvider(configName string) []string {
	return []string{"conf"}
}

func main() {
	observatory := observe.MakeObservatoryFromEnv()
	defer observatory.Close()

	span := observatory.StartTrace("main()")
	defer span.Finish()

	graphQLHTTPServer := server.CreateGraphQLOverHTTPServer(observatory, configPathProvider, span)
	//TODO: graphQLHTTPServer resolvers have configurations that need to be closed so call resolvers.Close()

	fmt.Printf("Listening on %s, serving configs from %v, try http://localhost%s/playground", graphQLHTTPServer.Addr, configPathProvider(""), graphQLHTTPServer.Addr)
	log.Fatal(graphQLHTTPServer.ListenAndServe())

	graphQLHTTPServer.Close()
}
