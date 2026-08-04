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

import React, {useCallback, useEffect, useState} from "react";
import {Alert, Space, Table, Tag, Tooltip, Typography} from "antd";
import {RobotOutlined} from "@ant-design/icons";
import {Link} from "react-router-dom";
import {getSessions} from "../backend/api";
import {AgentIcon} from "./policySetUtil";

const {Title, Text} = Typography;

export default function SessionsPage() {
  const [sessions, setSessions] = useState([]);
  const [error, setError] = useState(null);

  const load = useCallback(() => {
    getSessions()
      .then((data) => {
        setSessions(data || []);
        setError(null);
      })
      .catch((err) => setError(err.message));
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, [load]);

  const columns = [
    {
      title: "Session",
      key: "title",
      render: (_, session) => (
        <Space direction="vertical" size={0}>
          <Link to={`/records?session=${encodeURIComponent(session.sessionKey)}`}>{session.title}</Link>
          <Text type="secondary" style={{fontSize: 12}} ellipsis>{session.sessionKey}</Text>
        </Space>
      ),
    },
    {
      title: "Agent",
      dataIndex: "agent",
      key: "agent",
      render: (v) => (
        <Tag color="blue" style={{display: "inline-flex", alignItems: "center", gap: 6}}>
          <AgentIcon agent={v} size={16} fallback={<RobotOutlined />} />
          {v}
        </Tag>
      ),
    },
    {title: "Records", dataIndex: "recordCount", key: "recordCount"},
    {
      title: "Blocked",
      dataIndex: "blockedCount",
      key: "blockedCount",
      render: (value) => (value > 0 ? <Tag color="red">{value}</Tag> : null),
    },
    {title: "First activity", dataIndex: "firstTime", key: "firstTime", render: (v) => new Date(v).toLocaleString()},
    {title: "Last activity", dataIndex: "lastTime", key: "lastTime", render: (v) => new Date(v).toLocaleString()},
  ];

  return (
    <div>
      <Space style={{display: "flex", justifyContent: "space-between", marginBottom: 16}}>
        <Tooltip title="One row per session key seen across the behaviour records. The title is guessed from the first tool the session used, since aiguard never stores prompt text.">
          <Title level={3} style={{margin: 0}}>Sessions</Title>
        </Tooltip>
      </Space>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}
      <Table
        rowKey="sessionKey"
        dataSource={sessions}
        columns={columns}
        pagination={{pageSize: 20}}
        size="small"
        locale={{emptyText: "No sessions yet - patch an agent to start collecting them"}}
      />
    </div>
  );
}
