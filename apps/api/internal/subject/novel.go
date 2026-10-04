package subject

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"forum/internal/domain"
)

const novelBaseURL = "https://n.novelia.cc/api"

func Novel() Plugin {
	return novelPlugin(&http.Client{
		Timeout:       3 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}, novelBaseURL)
}

func novelPlugin(client *http.Client, baseURL string) Plugin {
	return Plugin{
		Kind:        "novel",
		SubjectType: domain.CommentSubjectNovel,
		Validate: func(key string) bool {
			_, ok := novelResolver(baseURL, key)
			return ok
		},
		Check: func(ctx context.Context, key string) bool {
			endpoint, ok := novelResolver(baseURL, key)
			if !ok {
				return false
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return false
			}
			response, err := client.Do(req)
			if err != nil {
				return false
			}
			defer response.Body.Close()
			return response.StatusCode == http.StatusNoContent
		},
	}
}

// Key formats: web-{providerId}-{novelId} or wenku-{novelId}.
// Web novel IDs may contain hyphens.
func novelResolver(base, key string) (string, bool) {
	if id, ok := strings.CutPrefix(key, "wenku-"); ok && id != "" {
		return base + "/wenku/" + url.PathEscape(id) + "/exist", true
	}
	parts := strings.SplitN(key, "-", 3)
	if len(parts) == 3 && parts[0] == "web" && parts[1] != "" && parts[2] != "" {
		return base + "/novel/" + url.PathEscape(parts[1]) + "/" + url.PathEscape(parts[2]) + "/exist", true
	}
	return "", false
}
