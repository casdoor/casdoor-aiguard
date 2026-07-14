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
import {Layout, Menu} from "antd";
import {DashboardOutlined, RobotOutlined, SafetyCertificateOutlined, SettingOutlined, ApiOutlined} from "@ant-design/icons";
import {Link, Route, Switch, useLocation} from "react-router-dom";
import DashboardPage from "./pages/DashboardPage";
import PolicyPage from "./pages/PolicyPage";
import InterceptPage from "./pages/InterceptPage";
import CasdoorSettingsPage from "./pages/CasdoorSettingsPage";
import AgentsPage from "./pages/AgentsPage";

const {Header, Sider, Content} = Layout;

const menuItems = [
  {key: "/", icon: <DashboardOutlined />, label: <Link to="/">Dashboard</Link>},
  {key: "/agents", icon: <RobotOutlined />, label: <Link to="/agents">Agents</Link>},
  {key: "/policy", icon: <SafetyCertificateOutlined />, label: <Link to="/policy">Policy</Link>},
  {key: "/intercept", icon: <ApiOutlined />, label: <Link to="/intercept">Interception</Link>},
  {key: "/casdoor", icon: <SettingOutlined />, label: <Link to="/casdoor">Casdoor Connection</Link>},
];

function App() {
  const location = useLocation();

  return (
    <Layout style={{minHeight: "100vh"}}>
      <Header style={{display: "flex", alignItems: "center"}}>
        <div style={{color: "#fff", fontSize: 18, fontWeight: 600}}>Casdoor AIGuard</div>
      </Header>
      <Layout>
        <Sider width={220} theme="light">
          <Menu mode="inline" selectedKeys={[location.pathname]} items={menuItems} style={{height: "100%"}} />
        </Sider>
        <Layout style={{padding: 24}}>
          <Content style={{background: "#fff", padding: 24, margin: 0}}>
            <Switch>
              <Route exact path="/" component={DashboardPage} />
              <Route exact path="/agents" component={AgentsPage} />
              <Route exact path="/policy" component={PolicyPage} />
              <Route exact path="/intercept" component={InterceptPage} />
              <Route exact path="/casdoor" component={CasdoorSettingsPage} />
            </Switch>
          </Content>
        </Layout>
      </Layout>
    </Layout>
  );
}

export default App;
