package main

import (
	"net/url"
	"testing"
)

func TestRouterLookup(t *testing.T) {
	r := newRouter("test.com")
	u, _ := url.Parse("http://10.0.0.1:80")
	r.set("hello", u)
	r.set("MixedCase", u)

	tests := []struct {
		name string
		host string
		hit  bool
	}{
		{"exact match", "hello.test.com", true},
		{"with port", "hello.test.com:8080", true},
		{"uppercase host", "HELLO.TEST.COM", true},
		{"mixed-case registered name still matches lowercase host", "mixedcase.test.com", true},
		{"unknown subdomain", "nope.test.com", false},
		{"wrong domain", "hello.other.com", false},
		{"bare domain has no container", "test.com", false},
		{"multi-level subdomain is rejected", "a.hello.test.com", false},
		{"empty host", "", false},
		{"single-label sub that isn't registered", "anyhello.test.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.lookup(tt.host) != nil
			if got != tt.hit {
				t.Errorf("lookup(%q): got hit=%v, want %v", tt.host, got, tt.hit)
			}
		})
	}
}

func TestRouterDelete(t *testing.T) {
	r := newRouter("test.com")
	u, _ := url.Parse("http://10.0.0.1:80")
	r.set("hello", u)
	if r.lookup("hello.test.com") == nil {
		t.Fatal("expected route to be set")
	}
	r.delete("hello")
	if r.lookup("hello.test.com") != nil {
		t.Fatal("expected route to be removed")
	}
	// deleting a non-existent route should not panic
	r.delete("never-existed")
}

func TestRouterDomainNormalization(t *testing.T) {
	// leading dot and uppercase should be normalized
	r := newRouter(".Test.COM")
	u, _ := url.Parse("http://10.0.0.1:80")
	r.set("hello", u)
	if r.lookup("hello.test.com") == nil {
		t.Fatal("expected lookup to succeed against normalized domain")
	}
}
