import React from 'react';
import { HashRouter, Switch, Route, Redirect } from 'react-router-dom';
import { hot } from 'react-hot-loader/root';
import { Page403, Page404 } from '@cpts/Exception';
import { Login, Register, PrivateRoute } from '@cpts/Auth';
import Layout from './components/Layout';
import Monitor from './pages/Monitor';
import ServiceTree from './pages/ServiceTree';
import User from './pages/User';
import Profile from './pages/Profile';

interface Props {
  habitsId: string;
}

function App({ habitsId }: Props) {
  const menuConf = [
    {
      name: '监控对象',
      path: 'sTree',
      icon: 'cluster',
      children: [
        {
          name: '全部对象',
          path: 'endpointMgmt',
        }, {
          name: '节点下对象',
          path: 'endpoints',
        }, {
          name: '树节点管理',
          path: 'node',
        },
      ],
    }, {
      name: '监控报警',
      path: 'monitor',
      icon: 'icon-speed-fast',
      children: [
        {
          name: '监控看图',
          path: 'dashboard',
        }, {
          name: '监控大盘',
          path: 'screen',
        }, {
          name: '报警策略',
          path: 'strategy',
        }, {
          name: '报警历史',
          path: 'history',
        }, {
          name: '报警屏蔽',
          path: 'silence',
        }, {
          name: '采集配置',
          path: 'collect',
        },
      ],
    }, {
      name: '用户管理',
      path: 'user',
      icon: 'icon-users2',
      children: [
        {
          name: '用户管理',
          path: 'list',
        }, {
          name: '团队管理',
          path: 'team',
        },
      ],
    },
  ];
  return (
    <HashRouter>
      <Switch>
        <Route path="/login" component={Login} />
        <Route path="/register" component={Register} />
        <Route path="/403" component={Page403} />
        <Route path="/404" component={Page404} />
        <Layout
          appName=""
          menuConf={menuConf}
          habitsId={habitsId}
        >
          <Switch>
            <Route exact path="/" render={() => <Redirect to="/sTree" />} />
            <PrivateRoute path="/monitor" component={Monitor} />
            <PrivateRoute path="/sTree" component={ServiceTree} />
            <PrivateRoute path="/user" component={User} />
            <PrivateRoute path="/profile" component={Profile} />
            <Route render={() => <Redirect to="/404" />} />
          </Switch>
        </Layout>
      </Switch>
    </HashRouter>
  );
}

export default hot(App);
