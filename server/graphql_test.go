// handlers_test.go
package server

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	observe "github.com/shah/observe-go"
	"github.com/stretchr/testify/suite"
)

var newLinesRegExp = regexp.MustCompile(`[\n\r]`)
var quotesRegExp = regexp.MustCompile(`["]`)

type GraphQLOverHTTPServerSuite struct {
	suite.Suite
	observatory observe.Observatory
	span        opentracing.Span
}

func (suite *GraphQLOverHTTPServerSuite) SetupSuite() {
	observatory := observe.MakeObservatoryFromEnv()
	suite.observatory = observatory
	suite.span = observatory.StartTrace("GraphQLOverHTTPServerSuite")
}

func (suite *GraphQLOverHTTPServerSuite) TearDownSuite() {
	suite.span.Finish()
	suite.observatory.Close()
}

func (suite *GraphQLOverHTTPServerSuite) TestHealthCheckHandler() {
	req, err := http.NewRequest("GET", "/health-check", nil)
	suite.Nil(err, "Unable to create request")

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(healthCheckHandler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	suite.Equal(http.StatusOK, rr.Code, "Invalid HTTP Status")
	suite.JSONEq(`{ "alive" : true }`, rr.Body.String(), "Unexpected response")
}

func cleanQuery(query []byte) string {
	text := fmt.Sprintf("%s", query)
	text = newLinesRegExp.ReplaceAllString(text, "")
	text = quotesRegExp.ReplaceAllString(text, `\"`)
	return text
}

func (suite *GraphQLOverHTTPServerSuite) testGraphQLQuery(queryName string) {
	queryFileName := fmt.Sprintf("test-data/query-%s.graphql", queryName)
	query, queryReadErr := ioutil.ReadFile(queryFileName)
	suite.Nilf(queryReadErr, "Unable to read query from file %s", queryFileName)

	responseToCompareToFileName := fmt.Sprintf("test-data/query-%s-response.json", queryName)
	responseToCompareTo, responseCompareReadErr := ioutil.ReadFile(responseToCompareToFileName)
	suite.Nilf(responseCompareReadErr, "Unable to read compare to response from file %s", responseToCompareToFileName)

	postBody := fmt.Sprintf(`{"query":"%s","variables":null}`, cleanQuery(query))
	req, err := http.NewRequest("POST", "/graphql", strings.NewReader(postBody))
	suite.Nil(err, "Unable to create request")

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(createExecutableSchemaHandler(suite.observatory, func(string) []string { return []string{"../conf"} }, suite.span))

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder with context info.
	ctx := opentracing.ContextWithSpan(req.Context(), suite.span)
	req = req.WithContext(ctx)
	handler.ServeHTTP(rr, req)

	suite.Equalf(http.StatusOK, rr.Code, "Invalid HTTP Status")
	suite.JSONEq(fmt.Sprintf("%s", responseToCompareTo), rr.Body.String(), "Unexpected response")
}

func (suite *GraphQLOverHTTPServerSuite) TestConfigGraphQLQuery() {
	suite.testGraphQLQuery("config")
}

func (suite *GraphQLOverHTTPServerSuite) TestConfigsGraphQLQuery() {
	suite.testGraphQLQuery("configs")
}

func (suite *GraphQLOverHTTPServerSuite) TestUrlsInTextGraphQLQuery() {
	suite.testGraphQLQuery("urlsInText")
}

func TestSuite(t *testing.T) {
	suite.Run(t, new(GraphQLOverHTTPServerSuite))
}
