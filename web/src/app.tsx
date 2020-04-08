import React, { useState } from 'react';
import { HashRouter, Switch, Route, Redirect } from 'react-router-dom';
import { hot } from 'react-hot-loader/root';
import { ConfigProvider } from 'antd';
import antdZhCN from 'antd/lib/locale/zh_CN';
import antdEnUS from 'antd/lib/locale/en_US';
import { IntlProvider } from 'react-intl';
import _ from 'lodash';
import { Page403, Page404 } from '@cpts/Exception';
import { Login, Register, PrivateRoute } from '@cpts/Auth';
import Layout from '@cpts/Layout';
import intlZhCN from './locales/zh';
import intlEnUS from './locales/en';
import Monitor from './pages/Monitor';
import ServiceTree from './pages/ServiceTree';
import User from './pages/User';
import Profile from './pages/Profile';

interface Props {
  habitsId: string;
}

interface LocaleMap {
  [index: string]: any,
}

const localeMap: LocaleMap = {
  zh: {
    antd: antdZhCN,
    intl: 'zh',
    intlMessages: intlZhCN,
  },
  en: {
    antd: antdEnUS,
    intl: 'en',
    intlMessages: intlEnUS,
  },
};
const defaultLanguage = window.localStorage.getItem('language') || navigator.language.substr(0, 2);

function App({ habitsId }: Props) {
  const [language, setLanguage] = useState(defaultLanguage);
  const intlMessages = _.get(localeMap[language], 'intlMessages', intlZhCN);
  const menuConf = [
    {
      name: intlMessages['menu.endpoints'],
      path: 'sTree',
      icon: 'cluster',
      children: [
        {
          name: intlMessages['menu.endpoints.all'],
          path: 'endpointMgmt',
        }, {
          name: intlMessages['menu.endpoints.node'],
          path: 'endpoints',
        }, {
          name: intlMessages['menu.endpoints.node.manage'],
          path: 'node',
        },
      ],
    }, {
      name: intlMessages['menu.monitor'],
      path: 'monitor',
      icon: 'icon-speed-fast',
      children: [
        {
          name: intlMessages['menu.monitor.dashboard'],
          path: 'dashboard',
        }, {
          name: intlMessages['menu.monitor.screen'],
          path: 'screen',
        }, {
          name: intlMessages['menu.monitor.strategy'],
          path: 'strategy',
        }, {
          name: intlMessages['menu.monitor.history'],
          path: 'history',
        }, {
          name: intlMessages['menu.monitor.silence'],
          path: 'silence',
        }, {
          name: intlMessages['menu.monitor.collect'],
          path: 'collect',
        },
      ],
    }, {
      name: intlMessages['menu.users'],
      path: 'user',
      icon: 'icon-users2',
      children: [
        {
          name: intlMessages['menu.users.users'],
          path: 'list',
        }, {
          name: intlMessages['menu.users.teams'],
          path: 'team',
        },
      ],
    },
  ];

  return (
    <IntlProvider
      locale={_.get(localeMap[language], 'intl', 'zh')}
      messages={intlMessages}
    >
      <ConfigProvider locale={_.get(localeMap[language], 'antd', antdZhCN)}>
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
              language={language}
              onLanguageChange={(newLanguage) => {
                setLanguage(newLanguage);
                window.localStorage.setItem('language', newLanguage);
              }}
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
      </ConfigProvider>
    </IntlProvider>
  );
}

export default hot(App);
