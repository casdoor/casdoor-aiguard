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

import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Form, Input, Modal, Popconfirm, Radio, Space, Spin, Table, Tag, Tooltip, Typography, message} from "antd";
import {ReloadOutlined, RobotOutlined} from "@ant-design/icons";
import {Link} from "react-router-dom";
import {getAgentLlmApi, getAgents, patchAgent, unpatchAgent, updateAgentLlmApi} from "../backend/api";
import {AgentIcon} from "./policySetUtil";

const {Title, Text} = Typography;

const rowKey = (record) => `${record.owner}:${record.path}`;

export default function AgentsPage() {
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  // The key of the row whose patch is in flight, so only its button spins.
  const [busyKey, setBusyKey] = useState(null);
  const [apiRecord, setApiRecord] = useState(null);
  const [apiConfig, setApiConfig] = useState(null);
  const [apiLoading, setApiLoading] = useState(false);
  const [apiSaving, setApiSaving] = useState(false);
  const [apiError, setApiError] = useState(null);
  const apiRequestId = useRef(0);

  const load = () => {
    setLoading(true);
    setError(null);
    getAgents()
      .then((data) => setAgents(data || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const togglePatch = (record) => {
    const patched = record.patched;
    const target = {agentId: record.agentId, path: record.path, owner: record.owner};

    setBusyKey(rowKey(record));
    (patched ? unpatchAgent(target) : patchAgent(target))
      .then(() => {
        // What the patch did, and what is still left for the user to do, are
        // the patcher's words: this page never branches on which agent it is.
        const success = patched ? `Unpatched ${record.name}` : `Patched ${record.name}`;
        message.success(record.followup ? `${success}. ${record.followup}` : success);
        load();
      })
      .catch((err) => message.error(err.message))
      .finally(() => setBusyKey(null));
  };

  const openApiConfig = (record) => {
    const requestId = ++apiRequestId.current;
    const target = {agentId: record.agentId, path: record.path, owner: record.owner};
    setApiRecord(record);
    setApiConfig(null);
    setApiError(null);
    setApiLoading(true);
    getAgentLlmApi(target)
      .then((config) => {
        if (apiRequestId.current === requestId) {
          setApiConfig({
            mode: config.mode || "official",
            baseUrl: config.baseUrl || "",
            model: config.model || "",
            hasApiKey: Boolean(config.hasApiKey),
            apiKey: "",
          });
        }
      })
      .catch((err) => {
        if (apiRequestId.current === requestId) {
          setApiError(err.message);
        }
      })
      .finally(() => {
        if (apiRequestId.current === requestId) {
          setApiLoading(false);
        }
      });
  };

  const saveApiConfig = () => {
    const baseUrl = apiConfig.baseUrl.trim();
    const model = apiConfig.model.trim();
    const apiKey = apiConfig.apiKey.trim();
    const isCodex = apiRecord.agentId === "codex" || apiRecord.agentId === "codex-cli";

    if (apiConfig.mode === "relay" && !baseUrl) {
      message.error("API endpoint is required for relay mode");
      return;
    }
    if (apiConfig.mode === "relay" && isCodex && !model) {
      message.error("Model is required for Codex relay mode");
      return;
    }
    if (apiConfig.mode === "relay" && !apiConfig.hasApiKey && !apiKey) {
      message.error("API key is required the first time relay mode is configured");
      return;
    }

    setApiSaving(true);
    setApiError(null);
    updateAgentLlmApi({
      agentId: apiRecord.agentId,
      path: apiRecord.path,
      owner: apiRecord.owner,
      mode: apiConfig.mode,
      baseUrl,
      model,
      apiKey,
    })
      .then(() => {
        message.success(`Saved ${apiRecord.name} API configuration. Start a new session or restart ${apiRecord.name} to load it.`);
        setApiRecord(null);
      })
      .catch((err) => setApiError(err.message))
      .finally(() => setApiSaving(false));
  };

  const renderStatus = (_, record) => {
    if (!record.supported) {
      return <Tooltip title={record.detail}><Tag>Not supported</Tag></Tooltip>;
    }
    const tag = record.patched ? <Tag color="green">Patched</Tag> : <Tag color="default">Not patched</Tag>;
    return record.detail ? <Tooltip title={record.detail}>{tag}</Tooltip> : tag;
  };

  const renderAction = (_, record) => {
    let patchButton = <Button size="small" disabled>Patch</Button>;
    if (record.supported) {
      const button = (
        <Button size="small" type={record.patched ? "default" : "primary"} loading={busyKey === rowKey(record)}>
          {record.patched ? "Unpatch" : "Patch"}
        </Button>
      );
      const description = [record.notice, record.followup].filter(Boolean).join(" ");
      patchButton = (
        <Popconfirm
          title={record.patched ? `Unpatch ${record.name}?` : `Patch ${record.name}?`}
          description={description}
          okText={record.patched ? "Unpatch" : "Patch"}
          onConfirm={() => togglePatch(record)}
        >
          {button}
        </Popconfirm>
      );
    }

    return (
      <Space size={8}>
        {patchButton}
        {record.apiConfigurable && <Button size="small" onClick={() => openApiConfig(record)}>Configure API</Button>}
      </Space>
    );
  };

  const columns = [
    {
      title: "Agent",
      dataIndex: "name",
      key: "name",
      // The brand mark is looked up from the agent's id, so an agent whose
      // display name the icon table does not know still shows its name alone.
      render: (value, record) => (
        <Space size={8}>
          <AgentIcon agent={record.agentId || value} size={20} fallback={<RobotOutlined style={{fontSize: 18, color: "#8c8c8c"}} />} />
          {value}
        </Space>
      ),
    },
    {title: "Version", dataIndex: "version", key: "version", render: (value) => value || "Unknown"},
    {title: "Install Method", dataIndex: "installMethod", key: "installMethod", render: (value) => <Tag>{value}</Tag>},
    {title: "Owner", dataIndex: "owner", key: "owner"},
    {title: "Path", dataIndex: "path", key: "path", render: (value) => <Text code>{value}</Text>},
    {title: "Patch Status", key: "patched", render: renderStatus},
    {title: "Records", key: "records", render: (_, record) => (
      record.patched ? <Link to={`/records?agent=${encodeURIComponent(record.agentId)}`}>View</Link> : null
    )},
    {title: "Action", key: "action", render: renderAction},
  ];

  return (
    <div>
      <Space style={{display: "flex", justifyContent: "space-between", marginBottom: 16}}>
        <Title level={3} style={{margin: 0}}>Agents</Title>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={load}>Scan</Button>
      </Space>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}
      <Table
        rowKey={rowKey}
        dataSource={agents}
        columns={columns}
        loading={loading}
        pagination={false}
        locale={{emptyText: "No supported agents found"}}
        size="small"
      />
      <Modal
        title={apiRecord ? `Configure API - ${apiRecord.name}` : "Configure API"}
        open={Boolean(apiRecord)}
        okText="Save"
        confirmLoading={apiSaving}
        okButtonProps={{disabled: apiLoading || !apiConfig}}
        onOk={saveApiConfig}
        onCancel={() => {
          if (!apiSaving) {
            apiRequestId.current++;
            setApiRecord(null);
          }
        }}
      >
        {apiLoading && <div style={{padding: 32, textAlign: "center"}}><Spin /></div>}
        {apiError && <Alert type="error" message={apiError} style={{marginBottom: 16}} />}
        {apiRecord && apiConfig && (
          <Form layout="vertical">
            <Form.Item label="Mode">
              <Radio.Group
                optionType="button"
                buttonStyle="solid"
                value={apiConfig.mode}
                options={[
                  {label: "Official", value: "official"},
                  {label: "Relay", value: "relay"},
                ]}
                onChange={(e) => setApiConfig({...apiConfig, mode: e.target.value})}
              />
            </Form.Item>
            {apiConfig.mode === "relay" && (
              <>
                <Form.Item label="API endpoint" required>
                  <Input
                    value={apiConfig.baseUrl}
                    placeholder="https://api.example.com"
                    onChange={(e) => setApiConfig({...apiConfig, baseUrl: e.target.value})}
                  />
                </Form.Item>
                <Form.Item
                  label="API key"
                  required={!apiConfig.hasApiKey}
                  extra={apiConfig.hasApiKey ? "A key is already configured. Leave this blank to keep it." : "Enter the API key for this relay."}
                >
                  <Input.Password
                    value={apiConfig.apiKey}
                    autoComplete="new-password"
                    placeholder={apiConfig.hasApiKey ? "Leave blank to keep the current key" : "Enter API key"}
                    onChange={(e) => setApiConfig({...apiConfig, apiKey: e.target.value})}
                  />
                </Form.Item>
                <Form.Item
                  label="Model"
                  required={apiRecord.agentId === "codex" || apiRecord.agentId === "codex-cli"}
                  extra={apiRecord.agentId === "claude-code" ? "Optional for Claude Code." : undefined}
                >
                  <Input
                    value={apiConfig.model}
                    placeholder="Model name"
                    onChange={(e) => setApiConfig({...apiConfig, model: e.target.value})}
                  />
                </Form.Item>
              </>
            )}
          </Form>
        )}
      </Modal>
    </div>
  );
}
