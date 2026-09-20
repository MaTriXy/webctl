package scrape

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const challengeToken = "f235c594bd35489e"

func challengePage(action string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>Reddit</title>
<script nonce="x">document.addEventListener("DOMContentLoaded",async function(){var e=document.forms[0],n=(e.onsubmit=function(t){return !0},await(async e=>e+e)("%s"));e.elements.namedItem("solution").value=n,e.requestSubmit()},{once:!0});</script>
</head><body><main><form hidden method="GET" action="%s">
<input type="hidden" name="solution" />
<input type="hidden" name="js_challenge" value="1"/>
<input type="hidden" name="jsc_token" value="tok123"/>
<input type="hidden" name="jsc_orig_r" value=""/>
</form></main></body></html>`, challengeToken, action)
}

const threadPage = `<html><body><h1>Thread title</h1>
<template id="deferred-comments"><shreddit-comment><div slot="comment"><p>first comment</p></div></shreddit-comment></template>
</body></html>`

func TestFetchSolvesRedditChallenge(t *testing.T) {
	const path = "/r/golang/comments/abc/title/"
	var challenges, solves atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := r.Cookie("session"); err == nil {
			_, _ = io.WriteString(w, threadPage)
			return
		}
		q := r.URL.Query()
		if !q.Has("solution") {
			challenges.Add(1)
			_, _ = io.WriteString(w, challengePage(path))
			return
		}
		solves.Add(1)
		if r.URL.Path != path {
			t.Errorf("solve path = %q, want %q", r.URL.Path, path)
		}
		want := url.Values{
			"solution": {challengeToken + challengeToken}, "js_challenge": {"1"},
			"jsc_token": {"tok123"}, "jsc_orig_r": {""}, "sort": {"top"},
		}
		if q.Encode() != want.Encode() {
			t.Errorf("solve query = %v, want %v", q, want)
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "1", Path: "/"})
		_, _ = io.WriteString(w, threadPage)
	}))
	t.Cleanup(srv.Close)

	f := &Fetcher{}
	got, err := f.Fetch(context.Background(), srv.URL+path+"?sort=top")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Thread title") || !strings.Contains(got, "first comment") {
		t.Errorf("content = %q", got)
	}
	// The session cookie from the solve skips the challenge on later fetches.
	if _, err := f.Fetch(context.Background(), srv.URL+"/r/golang/comments/def/other/"); err != nil {
		t.Fatal(err)
	}
	if challenges.Load() != 1 || solves.Load() != 1 {
		t.Errorf("challenges = %d, solves = %d; want 1 and 1", challenges.Load(), solves.Load())
	}
}

func TestFetchRejectsUnknownChallenge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// The form is there but the solver script changed shape.
		_, _ = io.WriteString(w, strings.Replace(challengePage("/x/"), "e=>e+e", "e=>e+'!'", 1))
	}))
	t.Cleanup(srv.Close)
	_, err := (&Fetcher{}).Fetch(context.Background(), srv.URL+"/x/")
	if !errors.Is(err, errRedditChallenge) {
		t.Errorf("err = %v, want errRedditChallenge", err)
	}
}

func TestSolveRedditChallengeResolvesAction(t *testing.T) {
	u, _ := url.Parse("https://www.reddit.com/r/golang/comments/abc/title/?sort=top")
	got, err := solveRedditChallenge(u, []byte(challengePage("/r/golang/comments/abc/title/")))
	if err != nil {
		t.Fatal(err)
	}
	solved, _ := url.Parse(got)
	if solved.Host != "www.reddit.com" || solved.Path != "/r/golang/comments/abc/title/" {
		t.Errorf("solved = %q", got)
	}
	q := solved.Query()
	if q.Get("solution") != challengeToken+challengeToken || q.Get("jsc_token") != "tok123" || q.Get("sort") != "top" {
		t.Errorf("query = %v", q)
	}
	if _, err := solveRedditChallenge(u, []byte("<html><body>not a challenge</body></html>")); !errors.Is(err, errRedditChallenge) {
		t.Errorf("non-challenge err = %v", err)
	}
}

func TestHTMLToTextKeepsTemplateContent(t *testing.T) {
	got, err := HTMLToText(strings.NewReader(threadPage))
	if err != nil {
		t.Fatal(err)
	}
	if got != "Thread title\n\nfirst comment" {
		t.Errorf("got %q", got)
	}
}
