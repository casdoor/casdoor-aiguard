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
import {Typography, Table, Button, Space, Modal, Form, Input, Popconfirm, message} from "antd";
import {PlusOutlined, DeleteOutlined, EditOutlined} from "@ant-design/icons";
import {getSettings, updateSettings} from "../backend/api";

const {Title, Text, Paragraph} = Typography;

// crypto.randomUUID() needs a secure context; fall back to something unique
// enough for a client-generated, otherwise-opaque id.
const newProviderId = () => (
  window.crypto && window.crypto.randomUUID
    ? window.crypto.randomUUID()
    : `provider-${Date.now()}-${Math.random().toString(16).slice(2)}`
);

export default function LLMProvidersPage() {
  const [settings, setSettings] = useState(null);
  const [saving, setSaving] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  // The provider being edited, or null while adding a new one.
  const [editing, setEditing] = useState(null);
  const [form] = Form.useForm();

  useEffect(() => {
    getSettings().then(setSettings).catch((err) => message.error(err.message));
  }, []);

  if (!settings) {
    return null;
  }

  const providers = settings.llm.providers;

  // Every add/edit/delete persists immediately, the same way Patch/Unpatch on
  // the Agents page take effect right away - there is no separate page-level
  // Save button to avoid clobbering a concurrent edit with a stale list.
  const save = (nextProviders) => {
    setSaving(true);
    return updateSettings({...settings, llm: {...settings.llm, providers: nextProviders}})
      .then((saved) => setSettings(saved))
      .finally(() => setSaving(false));
  };

  const openAdd = () => {
    setEditing(null);
    form.resetFields();
    setModalOpen(true);
  };

  const openEdit = (record) => {
    setEditing(record);
    // The server never sends the existing key back (see api.js), so reset
    // first: without it, a stale value typed into a previous edit could
    // still be sitting in the form's internal store and look like it came
    // from this provider.
    form.resetFields();
    form.setFieldsValue(record);
    setModalOpen(true);
  };

  const submit = () => {
    form.validateFields().then((values) => {
      const next = editing
        ? providers.map((p) => (p.id === editing.id ? {...editing, ...values} : p))
        : [...providers, {...values, id: newProviderId()}];
      save(next)
        .then(() => {
          message.success(editing ? "Provider updated" : "Provider added");
          setModalOpen(false);
        })
        .catch((err) => message.error(err.message));
    });
  };

  const remove = (record) => {
    save(providers.filter((p) => p.id !== record.id))
      .then(() => message.success(`Deleted ${record.name}`))
      .catch((err) => message.error(err.message));
  };

  const columns = [
    {title: "Name", dataIndex: "name", key: "name"},
    {title: "Base URL", dataIndex: "baseUrl", key: "baseUrl", render: (value) => <Text code>{value}</Text>},
    {title: "API Key", dataIndex: "hasApiKey", key: "hasApiKey", render: (value) => (value ? "••••••••" : <Text type="secondary">Not set</Text>)},
    {title: "Model", dataIndex: "model", key: "model", render: (value) => value || <Text type="secondary">Default</Text>},
    {
      title: "Action",
      key: "action",
      render: (_, record) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => openEdit(record)}>Edit</Button>
          <Popconfirm title={`Delete ${record.name}?`} okText="Delete" onConfirm={() => remove(record)}>
            <Button size="small" danger icon={<DeleteOutlined />}>Delete</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Space style={{display: "flex", justifyContent: "space-between", marginBottom: 16}}>
        <Title level={3} style={{margin: 0}}>LLM Providers</Title>
        <Button type="primary" icon={<PlusOutlined />} onClick={openAdd}>Add Provider</Button>
      </Space>
      <Paragraph type="secondary">
        Save the LLM API endpoints you want to switch between - your own Anthropic or OpenAI key, a company proxy,
        a compatible third-party endpoint - then pick one per agent from the Provider column on the Agents page.
        Supported agents: Claude Code, Claude Desktop (Windows), Codex and Codex CLI.
      </Paragraph>
      <Table
        rowKey="id"
        dataSource={providers}
        columns={columns}
        pagination={false}
        locale={{emptyText: "No LLM providers saved yet"}}
        size="small"
      />

      <Modal
        title={editing ? "Edit Provider" : "Add Provider"}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={submit}
        confirmLoading={saving}
        okText="Save"
        destroyOnClose
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="Name" rules={[{required: true, message: "Name is required"}]}>
            <Input placeholder="e.g. DeepSeek" />
          </Form.Item>
          <Form.Item name="baseUrl" label="Base URL" rules={[{required: true, message: "Base URL is required"}]}>
            <Input placeholder="https://api.example.com/anthropic" />
          </Form.Item>
          <Form.Item
            name="apiKey"
            label="API Key"
            rules={[{required: !editing || !editing.hasApiKey, message: "API key is required"}]}
            extra={editing && editing.hasApiKey ? "A key is already saved. Leave this blank to keep it." : undefined}
          >
            <Input.Password autoComplete="new-password" placeholder={editing && editing.hasApiKey ? "Leave blank to keep the current key" : "sk-..."} />
          </Form.Item>
          <Form.Item
            name="model"
            label="Model"
            extra="Optional for Claude Code / Claude Desktop (overrides ANTHROPIC_MODEL). Required to use this provider with Codex or Codex CLI."
          >
            <Input placeholder="e.g. claude-sonnet-4-5, or a Codex-compatible model name" />
          </Form.Item>
          <Form.Item
            name="smallFastModel"
            label="Small/Fast Model (optional)"
            extra="Claude Code / Claude Desktop only - overrides ANTHROPIC_SMALL_FAST_MODEL. Not used by Codex."
          >
            <Input placeholder="Overrides ANTHROPIC_SMALL_FAST_MODEL" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
