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
import {Typography, Form, Input, Button, message} from "antd";
import {getSettings, updateSettings} from "../backend/api";

const {Title} = Typography;

export default function CasdoorSettingsPage() {
  const [settings, setSettings] = useState(null);

  useEffect(() => {
    getSettings()
      .then(setSettings)
      .catch((err) => message.error(err.message));
  }, []);

  if (!settings) {
    return null;
  }

  const casdoor = settings.casdoor;

  const setCasdoor = (patch) => {
    setSettings({...settings, casdoor: {...casdoor, ...patch}});
  };

  const save = () => {
    updateSettings(settings)
      .then((saved) => {
        setSettings(saved);
        message.success("Casdoor connection settings saved");
      })
      .catch((err) => message.error(err.message));
  };

  return (
    <div>
      <Title level={3}>Casdoor Connection</Title>
      <Typography.Paragraph type="secondary">
        aiguard authenticates to Casdoor with its own client-credentials grant, then calls Casdoor's Casbin
        enforce API to decide allow/deny for every recognized sensitive intent.
      </Typography.Paragraph>

      <Form layout="vertical" style={{maxWidth: 480}}>
        <Form.Item label="Endpoint">
          <Input value={casdoor.endpoint} onChange={(e) => setCasdoor({endpoint: e.target.value})} placeholder="http://localhost:8000" />
        </Form.Item>
        <Form.Item label="Client ID">
          <Input value={casdoor.clientId} onChange={(e) => setCasdoor({clientId: e.target.value})} />
        </Form.Item>
        <Form.Item label="Client Secret">
          <Input.Password value={casdoor.clientSecret} onChange={(e) => setCasdoor({clientSecret: e.target.value})} />
        </Form.Item>
        <Form.Item label="Organization">
          <Input value={casdoor.organization} onChange={(e) => setCasdoor({organization: e.target.value})} />
        </Form.Item>
        <Form.Item label="Application">
          <Input value={casdoor.application} onChange={(e) => setCasdoor({application: e.target.value})} />
        </Form.Item>
      </Form>

      <Button type="primary" onClick={save}>
        Save
      </Button>
    </div>
  );
}
