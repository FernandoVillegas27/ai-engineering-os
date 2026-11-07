package controlplane

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControlPlaneEmbeddedUIRoutes(t *testing.T) {
	handler := (HTTPServer{}).Handler()

	for _, path := range []string{"/ui/", "/ui/app.css", "/ui/app.js"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, recorder.Code, recorder.Body.String())
		}
		if recorder.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s missing CSP", path)
		}
		if recorder.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s missing frame denial", path)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("root status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "/ui/" {
		t.Fatalf("root redirect=%q", location)
	}
}

func TestControlPlaneEmbeddedUIIsReadOnly(t *testing.T) {
	handler := (HTTPServer{}).Handler()

	request := httptest.NewRequest(http.MethodPost, "/ui/", strings.NewReader("mutation"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if allow := recorder.Header().Get("Allow"); allow != "GET, HEAD" {
		t.Fatalf("allow=%q", allow)
	}
}

func TestControlPlaneUIDoesNotShadowAPI(t *testing.T) {
	handler := (HTTPServer{}).Handler()

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("body=%s", body)
	}
}

func TestControlPlaneUIUsesSameOriginReadOnlyEndpoints(t *testing.T) {
	handler := (HTTPServer{}).Handler()

	request := httptest.NewRequest(http.MethodGet, "/ui/app.js", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	script := recorder.Body.String()
	for _, required := range []string{
		`api("/runs")`,
		`"/runs/" + encoded + "/state"`,
		`"/runs/" + encoded + "/events"`,
		`"/runs/" + encoded + "/messages"`,
		`"/runs/" + encoded + "/artifacts"`,
		`"/runs/" + encoded + "/promotion"`,
		`"/runs/" + encoded + "/controls"`,
		`new EventSource(`,
		`task.tool_operations`,
		`formatToolOperations`,
		`tool_approvals`,
		`approval.requested`,
		`approval.granted`,
		`approval.rejected`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("dashboard script missing %q", required)
		}
	}
	for _, forbidden := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if strings.Contains(script, `method: "`+forbidden+`"`) {
			t.Fatalf("dashboard unexpectedly contains mutating method %s", forbidden)
		}
	}
}
