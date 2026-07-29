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
import {Alert, Button, Popconfirm, Space, Table, Tag, Tooltip, Typography, message} from "antd";
import {ReloadOutlined, RobotOutlined} from "@ant-design/icons";
import {Link} from "react-router-dom";
import {getAgents, patchAgent, unpatchAgent} from "../backend/api";
import {AgentIcon} from "./policySetUtil";

const {Title, Text} = Typography;

const rowKey = (record) => `${record.owner}:${record.path}`;

export default function AgentsPage() {
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  // The key of the row whose patch is in flight, so only its button spins.
  const [busyKey, setBusyKey] = useState(null);

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
    const isHermes = record.agentId === "hermes-agent";
    const target = {agentId: record.agentId, path: record.path, owner: record.owner};

    setBusyKey(rowKey(record));
    (patched ? unpatchAgent(target) : patchAgent(target))
      .then(() => {
        const success = patched ? `Unpatched ${record.name}` : `Patched ${record.name}`;
        message.success(isHermes ? `${success}. Restart running Hermes processes to apply the change.` : success);
        load();
      })
      .catch((err) => message.error(err.message))
      .finally(() => setBusyKey(null));
  };

  const renderStatus = (_, record) => {
    if (!record.supported) {
      return <Tooltip title={record.detail}><Tag>Not supported</Tag></Tooltip>;
    }
    const tag = record.patched ? <Tag color="green">Patched</Tag> : <Tag color="default">Not patched</Tag>;
    return record.detail ? <Tooltip title={record.detail}>{tag}</Tooltip> : tag;
  };

  const renderAction = (_, record) => {
    if (!record.supported) {
      return <Button size="small" disabled>Patch</Button>;
    }

    const button = (
      <Button size="small" type={record.patched ? "default" : "primary"} loading={busyKey === rowKey(record)}>
        {record.patched ? "Unpatch" : "Patch"}
      </Button>
    );
    const isHermes = record.agentId === "hermes-agent";
    const description = isHermes
      ? (record.patched
        ? "Removes the metadata-only observer from the default profile. Running Hermes processes keep it loaded until restarted."
        : "Installs a metadata-only observer in the default profile. Restart an already-running Hermes process to begin recording.")
      : (record.patched
        ? "Removes aiguard's hooks and restores every file the patch changed."
        : "Installs aiguard's hooks so this agent streams its behaviour to Records.");
    return (
      <Popconfirm
        title={record.patched ? `Unpatch ${record.name}?` : `Patch ${record.name}?`}
        description={description}
        okText={record.patched ? "Unpatch" : "Patch"}
        onConfirm={() => togglePatch(record)}
      >
        {button}
      </Popconfirm>
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
    </div>
  );
}
