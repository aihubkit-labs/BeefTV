package aihubkit

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/generation"
)

func TestUploadTicketRetriesConnectionFailureBeforeOneUpload(t *testing.T) {
	c := newClient()
	tickets, uploads := 0, 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		tickets++
		if r.URL.Path != "/v1/temporary-files" {
			t.Fatalf("unexpected call: %s", r.URL.Path)
		}
		if tickets == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"upload_url":"https://8.8.8.8/put?signature=private","url":"https://8.8.8.8/get?signature=private","expires_in":600}}`))}, nil
	})
	c.uploadHTTP.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		uploads++
		body, _ := io.ReadAll(r.Body)
		if string(body) != "abc" || r.Header.Get("Authorization") != "" {
			t.Error("retry consumed file data or leaked the platform credential")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	link, _, err := c.upload(context.Background(), "fixture", "file.png", "image/png", 3, strings.NewReader("abc"))
	if err != nil || link == "" || tickets != 2 || uploads != 1 {
		t.Fatalf("tickets=%d uploads=%d err=%v", tickets, uploads, err)
	}
}

func TestUploadTicketFailureIsBoundedRedactedAndClassifiedAsNetworkPreparation(t *testing.T) {
	c := newClient()
	attempts := 0
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if trace := httptrace.ContextClientTrace(r.Context()); trace != nil {
			trace.GotConn(httptrace.GotConnInfo{})
			trace.WroteRequest(httptrace.WroteRequestInfo{})
		}
		return nil, &url.Error{Op: "POST", URL: "https://example.invalid/?secret=never-expose", Err: io.EOF}
	})
	_, _, err := c.upload(context.Background(), "fixture", "file.png", "image/png", 3, strings.NewReader("abc"))
	if err == nil || attempts != 3 || strings.Contains(err.Error(), "never-expose") || !strings.Contains(err.Error(), "等待响应时连接中断") {
		t.Fatalf("attempts=%d error=%v", attempts, err)
	}
	wrapped := fmt.Errorf("reference: %w", generation.RequestPreparationError{Cause: generation.ReferenceStoragePreparationError{Cause: err}})
	failure := generation.ClassifyError(wrapped)
	if failure.ProviderCode != "reference_preupload_failed" || failure.Action != "请检查网络或稍后重试" || failure.Uncertain || failure.BlocksManualRetry() {
		t.Fatalf("incorrect preparation failure: %+v", failure)
	}
}

func TestUploadTicketDoesNotRetryRejectionsCancellationOrInvalidCertificates(t *testing.T) {
	for _, kind := range []string{"unauthorized", "business rejection", "certificate", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := newClient()
			attempts := 0
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				attempts++
				switch kind {
				case "unauthorized":
					return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("private"))}, nil
				case "business rejection":
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":false,"message":"private"}`))}, nil
				case "certificate":
					return nil, &tls.CertificateVerificationError{Err: errors.New("private certificate detail")}
				default:
					cancel()
					return nil, io.EOF
				}
			})
			_, _, err := c.upload(ctx, "fixture", "file.png", "image/png", 3, strings.NewReader("abc"))
			if err == nil || attempts != 1 || strings.Contains(err.Error(), "private") {
				t.Fatalf("attempts=%d err=%v", attempts, err)
			}
			if kind == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation")
			}
		})
	}
}
