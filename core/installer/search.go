package installer

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

var searchGithubRe = regexp.MustCompile(`https:\/\/github\.com\/(\w+)\/(\w+)`)

// ImFeelingLuck tries to auto-detect a GitHub repo for the given phrase.
func ImFeelingLuck(phrase string) (user, project string, err error) {
	phrase += " site:github.com"
	v := url.Values{}
	v.Set("q", "! "+phrase)
	if user, project, err := captureRepoLocation("https://html.duckduckgo.com/html?" + v.Encode()); err == nil {
		return user, project, nil
	}
	v = url.Values{}
	v.Set("btnI", "")
	v.Set("q", phrase)
	if user, project, err := captureRepoLocation("https://www.google.com/search?" + v.Encode()); err == nil {
		return user, project, nil
	}
	return "", "", errors.New("not found")
}

// captureRepoLocation uses I'm-feeling-lucky and grabs the Location
// header from the 302, which contains the GitHub repo.
func captureRepoLocation(url string) (user, project string, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/81.0.4044.122 Safari/537.36")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return "", "", fmt.Errorf("request failed: %s", err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 3 {
		return "", "", fmt.Errorf("non-redirect response: %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	m := searchGithubRe.FindStringSubmatch(loc)
	if len(m) == 0 {
		return "", "", fmt.Errorf("github url not found in redirect: %s", loc)
	}
	return m[1], m[2], nil
}
