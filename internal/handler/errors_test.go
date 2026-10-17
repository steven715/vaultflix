package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func respond(t *testing.T, write func(c *gin.Context)) (int, model.ErrorResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	write(c)
	var body model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return w.Code, body
}

// The one table from domain errors to HTTP: the frontend (web/src/api/errors.ts
// ErrorCode) and the player (web/src/lib/hlsError.ts) depend on these.
func TestWriteError_MapsDomainErrors(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{model.ErrNotFound, http.StatusNotFound, model.CodeNotFound},
		{&model.NotFoundError{What: "tag"}, http.StatusNotFound, model.CodeNotFound},
		{model.ErrNotRemux, http.StatusNotFound, model.CodeNotFound},
		{model.ErrPathNotExist, http.StatusNotFound, model.CodeFileNotFound},
		{model.ErrPathNotAllowed, http.StatusForbidden, model.CodePathNotAllowed},
		{model.ErrInvalidInput, http.StatusBadRequest, model.CodeBadRequest},
		{&model.InvalidParamError{Message: "page must be a positive integer"}, http.StatusBadRequest, model.CodeBadRequest},
		{model.ErrAlreadyExists, http.StatusConflict, model.CodeAlreadyExists},
		{service.ErrUsernameAlreadyExists, http.StatusConflict, model.CodeUsernameTaken},
		{&service.JobLaneBusyError{RunningKind: service.JobKindBackfill}, http.StatusConflict, model.CodeJobLaneBusy},
		{model.ErrConflict, http.StatusConflict, model.CodeConflict},
		{model.ErrMediaSourceDisabled, http.StatusConflict, model.CodeSourceDisabled},
		{model.ErrCannotDisableAdmin, http.StatusBadRequest, model.CodeCannotDisableAdmin},
		{service.ErrInvalidCredentials, http.StatusUnauthorized, model.CodeUnauthorized},
		{model.ErrAccountDisabled, http.StatusForbidden, model.CodeForbidden},
		{model.ErrCodeNotFound, http.StatusUnprocessableEntity, model.CodeNoCode},
		{model.ErrKeyframeIndexFailed, http.StatusUnprocessableEntity, model.CodeKeyframeIndexFailed},
		{model.ErrStreamPreparing, http.StatusServiceUnavailable, model.CodeStreamNotReady},
		{errors.New("db down"), http.StatusInternalServerError, model.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.wantCode+" "+tt.err.Error(), func(t *testing.T) {
			status, body := respond(t, func(c *gin.Context) {
				writeError(c, fmt.Errorf("wrapped: %w", tt.err), "do the thing")
			})
			if status != tt.wantStatus || body.Error != tt.wantCode {
				t.Errorf("got %d %q, want %d %q", status, body.Error, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

// Messages are for API callers: they name the operation, and never leak the
// wrapped chain of an unexpected error.
func TestWriteError_Messages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"mapped error names the operation", model.ErrNotFound, "get user: not found"},
		{"a missing resource says which", &model.NotFoundError{What: "tag"}, "get user: tag not found"},
		{"an invalid parameter says which", &model.InvalidParamError{Message: "page must be a positive integer"}, "page must be a positive integer"},
		{"a busy lane names the Job holding it", &service.JobLaneBusyError{RunningKind: service.JobKindImport}, "匯入正在執行，完成或取消後再試"},
		{"an unexpected error hides its chain", errors.New("pq: connection refused at 10.0.0.5"), "get user failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, body := respond(t, func(c *gin.Context) { writeError(c, fmt.Errorf("wrapped: %w", tt.err), "get user") })
			if body.Message != tt.want {
				t.Errorf("message = %q, want %q", body.Message, tt.want)
			}
		})
	}
}

func TestBadRequest(t *testing.T) {
	status, body := respond(t, func(c *gin.Context) { badRequest(c, "source_id is required") })
	if status != http.StatusBadRequest || body.Error != model.CodeBadRequest || body.Message != "source_id is required" {
		t.Errorf("got %d %+v", status, body)
	}
}

// The frontend's ErrorCode (web/src/api/errors.ts) must list exactly the
// codes the API answers with (model/error_codes.go), or a page's error texts
// silently miss.
func TestErrorCodes_MatchTheFrontend(t *testing.T) {
	backend := backendErrorCodes(t)

	ts, err := os.ReadFile("../../web/src/api/errors.ts")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(ts), "export type ErrorCode =")
	if start < 0 {
		t.Fatal("no `export type ErrorCode =` union in web/src/api/errors.ts")
	}
	var frontend []string
	member := regexp.MustCompile(`^\s*\|\s*'([a-z_]+)'\s*$`)
	for _, line := range strings.Split(string(ts[start:]), "\n")[1:] {
		m := member.FindStringSubmatch(line)
		if m == nil {
			break // the union ends at the first line that is not "| 'code'"
		}
		frontend = append(frontend, m[1])
	}

	sort.Strings(backend)
	sort.Strings(frontend)
	if !reflect.DeepEqual(backend, frontend) {
		t.Errorf("error codes differ:\nbackend  %v\nfrontend %v", backend, frontend)
	}
}

// Every ErrorResponse names its code by constant, so the list above is
// complete: no handler, middleware or route table spells one out.
func TestErrorCodes_NeverSpelledOut(t *testing.T) {
	spelled := regexp.MustCompile(`Error:\s*"`)
	for _, dir := range []string{".", "../middleware"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if spelled.Match(src) {
				t.Errorf("%s spells out an error code; use a model.Code constant", f)
			}
		}
	}
}

func backendErrorCodes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../model/error_codes.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if i >= len(vs.Values) || !strings.HasPrefix(name.Name, "Code") {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					codes = append(codes, strings.Trim(lit.Value, `"`))
				}
			}
		}
	}
	return codes
}
