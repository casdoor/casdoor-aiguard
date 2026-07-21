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

// Casdoor's OAuth authorize URL, built by hand rather than through
// casdoor-js-sdk: aiguard only ever needs this one URL, and the code it
// returns is exchanged for a token on the server side.
export const redirectPath = "/callback";

export function getRedirectUri() {
  return `${window.location.origin}${redirectPath}`;
}

export function getSigninUrl(authConfig) {
  const params = new URLSearchParams({
    client_id: authConfig.clientId,
    response_type: "code",
    redirect_uri: getRedirectUri(),
    scope: "read",
    // Casdoor identifies the application through the state parameter.
    state: authConfig.application,
  });
  return `${authConfig.endpoint}/login/oauth/authorize?${params.toString()}`;
}

// The Casdoor-hosted profile page of the signed-in operator.
export function getMyProfileUrl(authConfig) {
  return `${authConfig.endpoint}/account`;
}

// Where to come back to once the login round-trip finishes.
export function rememberReturnPath() {
  sessionStorage.setItem("from", `${window.location.pathname}${window.location.search}`);
}

export function takeReturnPath() {
  const from = sessionStorage.getItem("from");
  sessionStorage.removeItem("from");
  return from === null || from === redirectPath ? "/" : from;
}
