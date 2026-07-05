// Copyright 2025 The Casdoor Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package routers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/beego/beego/v2/server/web"
	beecontext "github.com/beego/beego/v2/server/web/context"
)

// webBuildDir is the production frontend bundle produced by `yarn --cwd web build`.
const webBuildDir = "web/build"

// InitStatic serves aiguard's React single-page app from web/build so the whole
// product ships as one binary. Anything under /api is left to the API router;
// every other path serves the requested static file, falling back to
// index.html so client-side routes (e.g. /policy, /settings) resolve.
func InitStatic() {
	web.InsertFilter("/*", web.BeforeRouter, serveSPA, web.WithReturnOnOutput(true))
}

func serveSPA(ctx *beecontext.Context) {
	urlPath := ctx.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") || urlPath == "/api" {
		return // handled by the API router
	}

	indexPath := filepath.Join(webBuildDir, "index.html")

	// Serve a real asset when it exists and stays inside web/build; otherwise
	// hand back index.html for the SPA to route.
	target := filepath.Join(webBuildDir, filepath.Clean("/"+urlPath))
	if within(webBuildDir, target) {
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			http.ServeFile(ctx.ResponseWriter, ctx.Request, target)
			return
		}
	}
	http.ServeFile(ctx.ResponseWriter, ctx.Request, indexPath)
}

// within reports whether target is inside base, guarding against path traversal.
func within(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
