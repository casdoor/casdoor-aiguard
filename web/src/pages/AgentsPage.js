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
import {Alert, Button, Space, Table, Tag, Typography} from "antd";
import {ReloadOutlined} from "@ant-design/icons";
import {getAgents} from "../backend/api";

const {Title, Text} = Typography;

export default function AgentsPage() {
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const load = () => {
    setLoading(true);
    setError(null);
    getAgents()
      .then((data) => setAgents(data || []))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const columns = [
    {title: "Agent", dataIndex: "name", key: "name"},
    {title: "Version", dataIndex: "version", key: "version", render: (value) => value || "Unknown"},
    {title: "Install Method", dataIndex: "installMethod", key: "installMethod", render: (value) => <Tag>{value}</Tag>},
    {title: "Owner", dataIndex: "owner", key: "owner"},
    {title: "Path", dataIndex: "path", key: "path", render: (value) => <Text code>{value}</Text>},
  ];

  return (
    <div>
      <Space style={{display: "flex", justifyContent: "space-between", marginBottom: 16}}>
        <Title level={3} style={{margin: 0}}>Agents</Title>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={load}>Scan</Button>
      </Space>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}
      <Table
        rowKey={(record) => `${record.owner}:${record.path}`}
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
