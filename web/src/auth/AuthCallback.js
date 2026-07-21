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

import React, {useEffect, useState} from "react";
import {Button, Result, Spin, message} from "antd";
import {useHistory, useLocation} from "react-router-dom";
import {signin} from "../backend/api";
import {takeReturnPath} from "./AuthUtil";

// Casdoor redirects here with ?code=&state= after a successful login; the code
// is only useful to the server, which exchanges it for a token and opens the
// session cookie.
export default function AuthCallback({onSignedIn}) {
  const location = useLocation();
  const history = useHistory();
  const [error, setError] = useState(null);

  useEffect(() => {
    const params = new URLSearchParams(location.search);
    const code = params.get("code");
    const state = params.get("state");

    if (!code || !state) {
      setError("The login callback is missing the OAuth code or state");
      return;
    }

    signin(code, state)
      .then((claims) => {
        onSignedIn(claims);
        message.success(`Signed in as ${claims.displayName || claims.name}`);
        history.replace(takeReturnPath());
      })
      .catch((err) => setError(err.message));
  }, [location.search, history, onSignedIn]);

  if (error) {
    return (
      <Result
        status="error"
        title="Login failed"
        subTitle={error}
        extra={<Button type="primary" onClick={() => history.replace("/")}>Back to Dashboard</Button>}
      />
    );
  }

  return (
    <div style={{display: "flex", justifyContent: "center", alignItems: "center", gap: 12, padding: 48}}>
      <Spin size="large" />
      <span>Signing in...</span>
    </div>
  );
}
