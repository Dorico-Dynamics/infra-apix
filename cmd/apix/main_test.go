package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apix "github.com/Infra-Forge/infra-apix"
)

func TestRunGenerate(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	apix.RegisterRoute(&apix.RouteRef{
		Method: apix.MethodGet,
		Path:   "/health",
		Responses: map[int]*apix.ResponseRef{
			200: {},
		},
	})

	out := filepath.Join(root, "openapi.json")
	cfg := generateConfig{projectPath: root, outputPath: out, format: "json"}
	if err := runGenerate(context.Background(), cfg); err != nil {
		t.Fatalf("runGenerate failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "/health") {
		t.Fatalf("expected spec to contain route")
	}
}

func TestRunSpecGuardMatches(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	out := filepath.Join(root, "openapi.yaml")

	apix.RegisterRoute(&apix.RouteRef{
		Method: apix.MethodGet,
		Path:   "/ready",
		Responses: map[int]*apix.ResponseRef{
			200: {},
		},
	})

	cfg := generateConfig{projectPath: root, outputPath: out, format: "yaml"}
	if err := runGenerate(context.Background(), cfg); err != nil {
		toFail(t, "initial generate", err)
	}

	if err := runSpecGuard(context.Background(), cfg, ""); err != nil {
		toFail(t, "spec guard should pass", err)
	}
}

func TestRunSpecGuardDetectsDrift(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	out := filepath.Join(root, "docs", "openapi.yaml")

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		toFail(t, "mkdir", err)
	}

	if err := os.WriteFile(out, []byte("invalid"), 0o644); err != nil {
		toFail(t, "seed spec", err)
	}

	apix.RegisterRoute(&apix.RouteRef{
		Method: apix.MethodGet,
		Path:   "/ping",
		Responses: map[int]*apix.ResponseRef{
			200: {},
		},
	})

	cfg := generateConfig{projectPath: root, outputPath: out, format: "yaml"}
	err := runSpecGuard(context.Background(), cfg, "")
	if err == nil {
		toFail(t, "expected drift detection", errors.New("no error"))
	}
}

func TestRunCLIGenerateWritesSpec(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	out := filepath.Join(root, "openapi.yaml")

	apix.RegisterRoute(&apix.RouteRef{
		Method: apix.MethodGet,
		Path:   "/status",
		Responses: map[int]*apix.ResponseRef{
			200: {},
		},
	})

	args := []string{
		"generate",
		"-project", root,
		"-out", out,
		"-format", "yaml",
	}

	if err := runCLI(context.Background(), args); err != nil {
		toFail(t, "runCLI generate", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		toFail(t, "read generated spec", err)
	}

	if !strings.Contains(string(data), "/status") {
		toFail(t, "spec missing route", errors.New("route not found"))
	}
}

func TestRunCLISpecGuardWrapsError(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	outRel := filepath.Join("docs", "openapi.yaml")
	outAbs := filepath.Join(root, outRel)

	if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
		toFail(t, "mkdir", err)
	}
	if err := os.WriteFile(outAbs, []byte("stale"), 0o644); err != nil {
		toFail(t, "seed spec", err)
	}

	apix.RegisterRoute(&apix.RouteRef{
		Method: apix.MethodGet,
		Path:   "/drift",
		Responses: map[int]*apix.ResponseRef{
			200: {},
		},
	})

	args := []string{
		"spec-guard",
		"-project", root,
		"-out", outRel,
	}

	err := runCLI(context.Background(), args)
	if err == nil {
		toFail(t, "expected drift error", errors.New("no error"))
	}

	var cmdErr commandError
	if !errors.As(err, &cmdErr) {
		toFail(t, "expected commandError", err)
	}
	if cmdErr.command != "spec-guard" {
		toFail(t, "unexpected command name", errors.New(cmdErr.command))
	}
	if !strings.Contains(cmdErr.Error(), "spec drift detected") {
		toFail(t, "missing drift message", cmdErr)
	}
}

func toFail(t *testing.T, msg string, err error) {
	t.Helper()
	t.Fatalf("%s: %v", msg, err)
}

func TestRunGenerateNoRoutes(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	cfg := generateConfig{projectPath: root, outputPath: filepath.Join(root, "spec.json"), format: "json"}
	if err := runGenerate(context.Background(), cfg); err == nil {
		t.Fatalf("expected error when no routes are registered")
	}
}

func TestRunGenerateInvalidFormat(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/", Responses: map[int]*apix.ResponseRef{200: {}}})
	cfg := generateConfig{projectPath: root, outputPath: filepath.Join(root, "spec.txt"), format: "invalid"}
	if err := runGenerate(context.Background(), cfg); err == nil {
		t.Fatalf("expected error for unsupported format")
	}
}

func TestRunGenerateStdout(t *testing.T) {
	t.Cleanup(apix.ResetRegistry)
	root := t.TempDir()
	apix.RegisterRoute(&apix.RouteRef{Method: apix.MethodGet, Path: "/", Responses: map[int]*apix.ResponseRef{200: {}}})
	old := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = write
	cfg := generateConfig{projectPath: root, outputPath: filepath.Join(root, "unused"), format: "json", stdout: true}
	if err := runGenerate(context.Background(), cfg); err != nil {
		t.Fatalf("expected stdout generation to succeed: %v", err)
	}
	if err := write.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	var buf strings.Builder
	if _, err := io.Copy(&buf, read); err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}
	os.Stdout = old
	output := buf.String()
	if !strings.HasPrefix(output, doNotEditHeader+"\n{") {
		t.Fatalf("expected the header line, then the document, got %.120q", output)
	}
	if !strings.Contains(output, "/") {
		t.Fatalf("expected spec content in stdout output")
	}
}

func TestParseServers(t *testing.T) {
	servers := parseServers(" https://b.example.com , https://a.example.com ,,")
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0] != "https://a.example.com" || servers[1] != "https://b.example.com" {
		t.Fatalf("servers not trimmed/sorted: %v", servers)
	}
	if out := parseServers(""); out != nil {
		t.Fatalf("expected nil slice for empty input")
	}
}
