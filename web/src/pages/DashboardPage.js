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
import {Table, Tag, Typography, Alert} from "antd";
import {getEvents} from "../backend/api";

const {Title} = Typography;

const decisionColor = {
  allow: "green",
  deny: "red",
  "step-up": "orange",
};

export default function DashboardPage() {
  const [events, setEvents] = useState([]);
  const [error, setError] = useState(null);

  useEffect(() => {
    const load = () => {
      getEvents(200)
        .then((data) => setEvents(data || []))
        .catch((err) => setError(err.message));
    };
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, []);

  const columns = [
    {title: "Time", dataIndex: "timestamp", key: "timestamp", render: (v) => new Date(v).toLocaleString()},
    {title: "Source Agent", dataIndex: "sourceProcess", key: "sourceProcess", render: (v) => v || "(unknown)"},
    {title: "Destination", dataIndex: "destination", key: "destination"},
    {
      title: "Intent",
      key: "intent",
      render: (_, record) => {
        if (!record.intent) {
          return "(unrecognized)";
        }
        const i = record.intent;
        if (i.category === "payment") {
          return `payment ${i.amount || ""} ${i.currency || ""} -> ${i.recipient || "?"}`;
        }
        if (i.toolName) {
          return `mcp tool: ${i.toolName}`;
        }
        return i.action;
      },
    },
    {title: "Recognizer", dataIndex: "recognizer", key: "recognizer"},
    {
      title: "Decision",
      dataIndex: "decision",
      key: "decision",
      render: (v) => <Tag color={decisionColor[v] || "default"}>{v}</Tag>,
    },
    {title: "Reason", dataIndex: "reason", key: "reason"},
  ];

  return (
    <div>
      <Title level={3}>Event Stream</Title>
      {error && <Alert type="error" message={error} style={{marginBottom: 16}} />}
      <Table
        rowKey="id"
        dataSource={events}
        columns={columns}
        pagination={{pageSize: 20}}
        size="small"
      />
    </div>
  );
}
