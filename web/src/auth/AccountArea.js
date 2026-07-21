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

import React from "react";
import {Avatar, Button, Dropdown, Tooltip, message} from "antd";
import {LoginOutlined, LogoutOutlined, UserOutlined} from "@ant-design/icons";
import {signout} from "../backend/api";
import {getMyProfileUrl, getSigninUrl, rememberReturnPath} from "./AuthUtil";

// The top-right corner of the layout: a login button while anonymous, the
// signed-in operator otherwise. Signing in is optional everywhere in aiguard,
// it only puts a name on who is operating the guard.
export default function AccountArea({authConfig, account, onSignedOut}) {
  if (authConfig === null) {
    return null;
  }

  if (!authConfig.available) {
    return (
      <Tooltip title="Configure the Casdoor connection to enable login">
        <Button type="text" disabled icon={<LoginOutlined />} style={{color: "rgba(255, 255, 255, 0.45)"}}>
          Login
        </Button>
      </Tooltip>
    );
  }

  if (account === null) {
    const login = () => {
      rememberReturnPath();
      window.location.href = getSigninUrl(authConfig);
    };

    return (
      <Button type="primary" icon={<LoginOutlined />} onClick={login}>
        Login
      </Button>
    );
  }

  const items = [
    {key: "profile", icon: <UserOutlined />, label: "My Profile"},
    {key: "signout", icon: <LogoutOutlined />, label: "Logout"},
  ];

  const onClick = ({key}) => {
    if (key === "profile") {
      window.open(getMyProfileUrl(authConfig), "_blank", "noopener,noreferrer");
      return;
    }

    signout()
      .then(() => {
        onSignedOut();
        message.success("Signed out");
      })
      .catch((err) => message.error(err.message));
  };

  return (
    <Dropdown menu={{items, onClick}} placement="bottomRight">
      <span style={{color: "#fff", cursor: "pointer", display: "inline-flex", alignItems: "center", gap: 8}}>
        <Avatar size="small" src={account.avatar} icon={<UserOutlined />} />
        {account.displayName || account.name}
      </span>
    </Dropdown>
  );
}
