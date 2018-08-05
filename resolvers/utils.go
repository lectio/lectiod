package resolvers

import (
	"net/url"

	"github.com/lectio/harvester"
	"github.com/lectio/lectiod/schema"
)

func resourceToString(hr *harvester.HarvestedResource) schema.URLText {
	if hr == nil {
		return ""
	}

	referrerURL, _, _ := hr.GetURLs()
	return urlToString(referrerURL)
}

func urlToString(url *url.URL) schema.URLText {
	if url == nil {
		return ""
	}
	return schema.URLText(url.String())
}
