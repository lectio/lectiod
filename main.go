package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/lectio/lectiod/resolvers"
	schema "github.com/lectio/lectiod/schema_defn"
	"github.com/lectio/lectiod/storage"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	otlog "github.com/opentracing/opentracing-go/log"
	observe "github.com/shah/observe-go"
	"github.com/vektah/gqlgen/graphql"
	"github.com/vektah/gqlgen/handler"
)

func main() {
	observatory := observe.MakeObservatoryFromEnv()
	defer observatory.Close()

	resolverMiddleware := func(ctx context.Context, next graphql.Resolver) (interface{}, error) {
		rctx := graphql.GetResolverContext(ctx)
		span, ctx := observatory.StartTraceFromContext(ctx, rctx.Object+" Handler Middleware",
			opentracing.Tag{Key: "resolver.object", Value: rctx.Object},
			opentracing.Tag{Key: "resolver.field", Value: rctx.Field.Name},
		)
		defer span.Finish()
		ext.SpanKind.Set(span, "server")
		ext.Component.Set(span, "gqlgen")
		res, err := next(ctx)
		if err != nil {
			ext.Error.Set(span, true)
			span.LogFields(
				otlog.String("event", "error"),
				otlog.String("message", err.Error()),
				otlog.Error(err),
			)
		}

		return res, err
	}

	requestMiddleware := func(ctx context.Context, next func(ctx context.Context) []byte) []byte {
		requestContext := graphql.GetRequestContext(ctx)
		span, ctx := observatory.StartTraceFromContext(ctx, "HTTP Request")
		defer span.Finish()
		span.LogFields(otlog.String("rawQuery", requestContext.RawQuery))
		// TODO ext.HTTPMethod.Set(span, ...)
		// TODO ext.HTTPUrl.Set(span, ...)
		ext.SpanKind.Set(span, "server")
		ext.Component.Set(span, "gqlgen")
		res := next(ctx)
		return res
	}

	storage := storage.NewFileStorage("./tmp/diskv_data")
	resolvers := resolvers.NewSchemaResolvers(observatory, storage)
	http.Handle("/", handler.Playground("Lectio", "/graphql"))
	http.Handle("/graphql", handler.GraphQL(schema.MakeExecutableSchema(resolvers),
		handler.ResolverMiddleware(resolverMiddleware), handler.RequestMiddleware(requestMiddleware)))

	// TODO Add Voyager documentation handler: https://github.com/APIs-guru/graphql-voyager

	fmt.Println("Listening on :8080/graphql, saving to " + resolvers.DefaultConfiguration().Settings().Storage.Filesys.BasePath)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
