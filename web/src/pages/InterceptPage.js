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
import {Typography, Form, InputNumber, Switch, Select, Button, message, Alert, Space} from "antd";
import {DownloadOutlined} from "@ant-design/icons";
import {getSettings, updateSettings, caCertDownloadUrl} from "../backend/api";

const {Title, Paragraph} = Typography;

export default function InterceptPage() {
  const [settings, setSettings] = useState(null);

  useEffect(() => {
    getSettings()
      .then(setSettings)
      .catch((err) => message.error(err.message));
  }, []);

  if (!settings) {
    return null;
  }

  const intercept = settings.intercept;

  const setIntercept = (patch) => {
    setSettings({...settings, intercept: {...intercept, ...patch}});
  };

  const save = () => {
    updateSettings(settings)
      .then((saved) => {
        setSettings(saved);
        message.success("Interception settings saved (restart aiguard for the proxy port to take effect)");
      })
      .catch((err) => message.error(err.message));
  };

  return (
    <div>
      <Title level={3}>Interception</Title>

      <Alert
        type="info"
        showIcon
        style={{marginBottom: 24}}
        message="Trust aiguard's CA on this host (or bake it into your agents' container base image), then run scripts/setup_iptables.sh as root to start redirecting egress HTTP(S) into aiguard."
      />

      <Form layout="vertical" style={{maxWidth: 480}}>
        <Form.Item label="Transparent proxy port">
          <InputNumber
            style={{width: "100%"}}
            value={intercept.proxyPort}
            onChange={(v) => setIntercept({proxyPort: v})}
          />
        </Form.Item>

        <Form.Item label="Fail-closed on Casdoor (PDP) unreachable" tooltip="Recognized sensitive operations are denied when Casdoor can't be reached">
          <Switch checked={intercept.failClosedOnPdpError} onChange={(v) => setIntercept({failClosedOnPdpError: v})} />
        </Form.Item>

        <Form.Item label="Passthrough unrecognized traffic" tooltip="Ordinary traffic no recognizer understood is allowed through untouched">
          <Switch checked={intercept.passthroughUnrecognized} onChange={(v) => setIntercept({passthroughUnrecognized: v})} />
        </Form.Item>

        <Form.Item label="Step-up default action" tooltip="Verdict applied while CIBA (human approval) is stubbed in stage 1">
          <Select
            value={intercept.stepUpDefaultAction}
            options={[{label: "deny", value: "deny"}, {label: "allow", value: "allow"}]}
            onChange={(v) => setIntercept({stepUpDefaultAction: v})}
          />
        </Form.Item>
      </Form>

      <Space>
        <Button type="primary" onClick={save}>
          Save
        </Button>
        <Button icon={<DownloadOutlined />} href={caCertDownloadUrl()}>
          Download CA Certificate
        </Button>
      </Space>

      <Paragraph type="secondary" style={{marginTop: 24}}>
        See scripts/setup_iptables.sh / scripts/setup_nftables.sh (must run as root on the Linux host).
      </Paragraph>
    </div>
  );
}
