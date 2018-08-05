package resolvers

import (
	"net/url"

	"github.com/lectio/harvester"
	"github.com/lectio/lectiod/models"
)

func resourceToString(hr *harvester.HarvestedResource) models.URLText {
	if hr == nil {
		return ""
	}

	referrerURL, _, _ := hr.GetURLs()
	return urlToString(referrerURL)
}

func urlToString(url *url.URL) models.URLText {
	if url == nil {
		return ""
	}
	return models.URLText(url.String())
}
