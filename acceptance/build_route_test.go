package acceptance

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

const (
	routeVersion  = "0.2.0"
	routeDirtyTag = "-dirty"
)

func flagsFor(version, commit, date string) string {
	var flags []string
	for _, each := range []struct{ name, value string }{{"version", version}, {"commit", commit}, {"date", date}} {
		if each.value != "" {
			flags = append(flags, "-X main."+each.name+"="+each.value)
		}
	}
	return strings.Join(flags, " ")
}

func getBuild(t *testing.T, w *world, authorization *string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("GET", w.url+"/v1/build", nil)
	if err != nil {
		t.Fatal(err)
	}
	if authorization != nil {
		req.Header.Set("Authorization", *authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func bearer(token string) *string {
	value := "Bearer " + token
	return &value
}

func buildFields(t *testing.T, body string) map[string]string {
	t.Helper()
	var generic map[string]any
	if err := json.Unmarshal([]byte(body), &generic); err != nil {
		t.Fatalf("the build answer %q is not a JSON object: %v", body, err)
	}
	fields := map[string]string{}
	for key, value := range generic {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("the build answer %q has a value of %s that is not a string", body, key)
		}
		fields[key] = text
	}
	if len(fields) != 3 {
		t.Fatalf("the build answer must have exactly version, commit, and date: %q", body)
	}
	for _, key := range []string{"version", "commit", "date"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("the build answer lacks %q: %q", key, body)
		}
	}
	return fields
}

func TestGetBuildAnswersTheSecretAndAnObserverTokenWithExactlyTheStampedVersionCommitAndDate(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit, stampDate)
	w := startHub(t, bin, t.TempDir(), "")
	observer := w.registerObserver("tui")
	for name, token := range map[string]string{"the secret": secret, "an observer token": observer} {
		status, body := getBuild(t, w, bearer(token))
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", name, status, body)
		}
		fields := buildFields(t, body)
		if fields["version"] != routeVersion || fields["commit"] != stampCommit || fields["date"] != stampDate {
			t.Errorf("%s: %v", name, fields)
		}
		if !commitForm.MatchString(fields["commit"]) || !dateForm.MatchString(fields["date"]) {
			t.Errorf("%s: the shapes are wrong: %v", name, fields)
		}
	}
}

func TestGetBuildAnswersTheDirtyCommitAsStampedAndAnEmptyDate(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit+routeDirtyTag, "")
	w := startHub(t, bin, t.TempDir(), "")
	status, body := getBuild(t, w, bearer(secret))
	fields := buildFields(t, body)
	if status != http.StatusOK || fields["version"] != routeVersion || fields["commit"] != stampCommit+routeDirtyTag || fields["date"] != "" {
		t.Fatalf("%d %v", status, fields)
	}
}

func TestGetBuildAnswersEmptyStringsForAVersionOnlyBuildAndDevForAPlainGoBuild(t *testing.T) {
	versionOnly := startHub(t, buildStamped(t, routeVersion, "", ""), t.TempDir(), "")
	status, body := getBuild(t, versionOnly, bearer(secret))
	fields := buildFields(t, body)
	if status != http.StatusOK || fields["version"] != routeVersion || fields["commit"] != "" || fields["date"] != "" {
		t.Errorf("a build with a version only: %d %v", status, fields)
	}
	plain := startHub(t, buildAllWith(t, ""), t.TempDir(), "")
	status, body = getBuild(t, plain, bearer(secret))
	fields = buildFields(t, body)
	if status != http.StatusOK || fields["version"] != "dev" || fields["commit"] != "" || fields["date"] != "" {
		t.Errorf("a plain go build: %d %v", status, fields)
	}
}

func TestGetBuildRefusesAnAgentTokenWith403AndNamesNoBuild(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit, stampDate)
	w := startHub(t, bin, t.TempDir(), "")
	w.register("alpha")
	status, body := getBuild(t, w, bearer(w.token("alpha")))
	if status != http.StatusForbidden {
		t.Fatalf("%d %s", status, body)
	}
	if strings.Contains(body, stampCommit) || strings.Contains(body, stampDate) {
		t.Errorf("the refusal carries the build: %s", body)
	}
}

func TestGetBuildRefusesAMissingOrWrongOrMalformedCredentialWith401(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit, stampDate)
	w := startHub(t, bin, t.TempDir(), "")
	w.register("alpha")
	basic := "Basic " + secret
	bare := secret
	empty := ""
	for name, authorization := range map[string]*string{
		"no header":                 nil,
		"an empty header":           &empty,
		"an empty bearer":           bearer(""),
		"a wrong secret":            bearer(secret + "x"),
		"an unknown token":          bearer("not-a-token-0123456789"),
		"the secret as a prefix":    bearer(secret[:len(secret)-1]),
		"the wrong scheme":          &basic,
		"the secret with no scheme": &bare,
	} {
		status, body := getBuild(t, w, authorization)
		if status != http.StatusUnauthorized {
			t.Errorf("%s: %d %s", name, status, body)
		}
		if strings.Contains(body, stampCommit) || strings.Contains(body, routeVersion) {
			t.Errorf("%s: the refusal carries the build: %s", name, body)
		}
	}
}

func TestGetBuildNeverEchoesTheSecretOrAnyTokenAndOnlyReadsWithGet(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit, stampDate)
	w := startHub(t, bin, t.TempDir(), "")
	w.register("alpha")
	observer := w.registerObserver("tui")
	agent := w.token("alpha")
	for _, token := range []string{secret, observer} {
		_, body := getBuild(t, w, bearer(token))
		for _, forbidden := range []string{secret, observer, agent} {
			if strings.Contains(body, forbidden) {
				t.Errorf("the build answer contains a credential: %s", body)
			}
		}
	}
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		status, body := w.request(method, "/v1/build", secret, map[string]string{}, nil)
		if status == http.StatusOK {
			t.Errorf("%s /v1/build answered 200: %s", method, body)
		}
	}
	if status, _ := getBuild(t, w, bearer(secret)); status != http.StatusOK {
		t.Errorf("the route stopped answering after the writes: %d", status)
	}
}

func TestGetBuildDoesNotRegisterOrListAnythingAndTheObserverKeepsItsReadOnlyRights(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit, stampDate)
	w := startHub(t, bin, t.TempDir(), "")
	observer := w.registerObserver("tui")
	for _, token := range []string{observer, secret} {
		if status, body := getBuild(t, w, bearer(token)); status != http.StatusOK {
			t.Fatalf("GET /v1/build: %d %s", status, body)
		}
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(w.agentsBody(observer)), &listed); err != nil || len(listed) != 0 {
		t.Errorf("asking for the build changed the agent list: %v %v", listed, err)
	}
	if status, _ := w.post("/v1/register", observer, map[string]string{"id": "mallory", "harness": "claude-code", "host": "mac"}); status != http.StatusForbidden {
		t.Errorf("the observer token may register: %d", status)
	}
}

func TestAnObserverTokenGets403OnEveryRouteExceptTheAgentListAndTheBuildWhichStayAt200(t *testing.T) {
	bin := buildStamped(t, routeVersion, stampCommit, stampDate)
	w := startHub(t, bin, t.TempDir(), "")
	w.register("alpha")
	observer := w.registerObserver("tui")
	for _, route := range []struct{ method, path string }{
		{"POST", "/v1/register"}, {"POST", "/v1/observers"}, {"POST", "/v1/send"}, {"GET", "/v1/stream"},
		{"POST", "/v1/deliveries/m-1/report"}, {"POST", "/v1/attach"}, {"GET", "/v1/log"},
	} {
		var body any
		if route.method == "POST" {
			body = map[string]string{}
		}
		if status, text := w.request(route.method, route.path, observer, body, nil); status != http.StatusForbidden {
			t.Errorf("%s %s with an observer token: %d %s", route.method, route.path, status, text)
		}
	}
	for _, path := range []string{"/v1/agents", "/v1/build"} {
		if status, text := w.request("GET", path, observer, nil, nil); status != http.StatusOK {
			t.Errorf("GET %s with an observer token: %d %s", path, status, text)
		}
	}
}

func TestAnObserverOfKindMenubarGetsAToken200AndTheTokenReadsTheAgentList(t *testing.T) {
	w := start(t)
	w.register("alpha")
	status, body := w.post("/v1/observers", secret, map[string]string{"kind": "menubar"})
	var issued struct{ Token string }
	if status != http.StatusOK || json.Unmarshal([]byte(body), &issued) != nil || issued.Token == "" {
		t.Fatalf("POST /v1/observers with kind menubar: %d %s", status, body)
	}
	var listed []map[string]any
	if err := json.Unmarshal([]byte(w.agentsBody(issued.Token)), &listed); err != nil || len(listed) != 1 || listed[0]["id"] != "alpha" {
		t.Errorf("the menubar token does not read the agent list: %v %v", listed, err)
	}
}
