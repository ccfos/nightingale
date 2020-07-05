import React, { Component } from 'react';
import { RouteComponentProps } from 'react-router-dom';
import PropTypes from 'prop-types';
import { Link, withRouter } from 'react-router-dom';
import { Layout, Dropdown, Menu, Icon, Button } from 'antd';
import classNames from 'classnames';
import PubSub from 'pubsub-js';
import _ from 'lodash';
import queryString from 'query-string';
import { WrappedComponentProps, injectIntl } from 'react-intl';
import { auth } from '@cpts/Auth';
import { MenuConfItem, TreeNode } from '@interface';
import request from '@common/request';
import api from '@common/api';
import { appname } from '@common/config';
import LayoutMenu from './LayoutMenu';
import NsTree from './NsTree';
import { normalizeTreeData } from './utils';
import './style.less';

interface Props {
  habitsId: string,
  appName: string,
  menuConf: MenuConfItem[],
  children: React.ReactNode,
  language: string,
  onLanguageChange: (language: string) => void,
}

interface State {
  checkAuthenticateLoading: boolean,
  nsTreeVisible: boolean,
  selectedNode: TreeNode | undefined,
  treeData: TreeNode[],
  originTreeData: TreeNode[],
  treeLoading: boolean,
  treeSearchValue: string,
  expandedKeys: string[],
  collapsed: boolean,
}

const { Header, Content, Sider } = Layout;

class NILayout extends Component<Props & RouteComponentProps & WrappedComponentProps, State> {
  static childContextTypes = {
    nsTreeVisibleChange: PropTypes.func.isRequired,
    getNodes: PropTypes.func.isRequired,
    selecteNode: PropTypes.func.isRequired,
    getSelectedNode: PropTypes.func.isRequired,
    updateSelectedNode: PropTypes.func.isRequired,
    deleteSelectedNode: PropTypes.func.isRequired,
    reloadNsTree: PropTypes.func.isRequired,
    habitsId: PropTypes.string.isRequired,
    intl: PropTypes.any.isRequired,
  };

  constructor(props: Props & RouteComponentProps & WrappedComponentProps) {
    super(props);
    let selectedNode;
    try {
      const selectedNodeStr = window.localStorage.getItem('selectedNode');
      if (selectedNodeStr) {
        selectedNode = JSON.parse(selectedNodeStr);
      }
    } catch (e) {
      console.log(e);
    }
    this.state = {
      checkAuthenticateLoading: true,
      nsTreeVisible: false,
      selectedNode,
      treeData: [],
      originTreeData: [],
      treeLoading: false,
      treeSearchValue: '',
      expandedKeys: [],
      collapsed: false,
    };
  }

  componentDidMount = () => {
    this.checkAuthenticate();
    this.fetchTreeData((treeData: TreeNode[]) => {
      this.getDefaultKeys(treeData);
    });
  }

  checkAuthenticate() {
    auth.checkAuthenticate().then(() => {
      this.setState({ checkAuthenticateLoading: false });
    });
  }

  fetchTreeData(cbk?: (treeData: TreeNode[]) => void) {
    const { treeSearchValue } = this.state;
    const url = treeSearchValue ? api.treeSearch : api.tree;
    const searchQuery = treeSearchValue ? { query: treeSearchValue } : undefined;
    this.setState({ treeLoading: true });
    request(`${url}?${searchQuery ? queryString.stringify(searchQuery) : ''}`).then((res) => {
      const treeData = normalizeTreeData(_.cloneDeep(res));
      this.setState({ treeData, originTreeData: res });
      if (treeSearchValue) {
        this.setState({ expandedKeys: _.map(res, n => _.toString(n.id)) });
      }
      if (cbk) cbk(res);
    }).finally(() => {
      this.setState({ treeLoading: false });
    });
  }

  getDefaultKeys(treeData: TreeNode[]) {
    const { selectedNode } = this.state;
    const selectedNodeId = _.get(selectedNode, 'id');
    const defaultExpandedKeys: string[] = [];

    function realFind(nid: number) {
      const node = _.find(treeData, { id: nid });
      if (node) {
        defaultExpandedKeys.push(_.toString(node.pid));
        if (node.pid !== 0) {
          realFind(node.pid);
        }
      }
    }

    if (selectedNodeId) realFind(selectedNodeId);
    this.setState({ expandedKeys: defaultExpandedKeys });
  }

  getChildContext() {
    return {
      nsTreeVisibleChange: (visible: boolean) => {
        this.setState({
          nsTreeVisible: visible,
        });
      },
      getNodes: () => {
        return _.cloneDeep(this.state.originTreeData);
      },
      selecteNode: (node: TreeNode) => {
        if (node) {
          try {
            window.localStorage.setItem('selectedNode', JSON.stringify(node));
          } catch (e) {
            console.log(e);
          }
          this.setState({ selectedNode: node });
        }
      },
      getSelectedNode: (key: string) => {
        const { originTreeData, selectedNode } = this.state;

        if (selectedNode && _.isPlainObject(selectedNode)) {
          if (_.find(originTreeData, { id: selectedNode.id })) {
            if (!key) {
              return { ...selectedNode };
            }
            return _.get(selectedNode, key);
          }
          return undefined;
        }
        return undefined;
      },
      updateSelectedNode: (node: TreeNode) => {
        try {
          window.localStorage.setItem('selectedNode', JSON.stringify(node));
        } catch (e) {
          console.log(e);
        }
        this.setState({ selectedNode: node });
      },
      deleteSelectedNode: () => {
        try {
          window.localStorage.removeItem('selectedNode');
        } catch (e) {
          console.log(e);
        }
        this.setState({ selectedNode: undefined });
      },
      reloadNsTree: () => {
        this.fetchTreeData();
      },
      habitsId: this.props.habitsId,
      intl: this.props.intl,
    };
  }

  handleLogoutLinkClick = () => {
    auth.signout(() => {
      this.props.history.push({
        pathname: '/',
      });
    });
  }

  handleNsTreeVisibleChange = (visible: boolean) => {
    this.setState({ nsTreeVisible: visible });
  }

  renderContent() {
    const prefixCls = `${appname}-layout`;
    const { nsTreeVisible } = this.state;
    const layoutCls = classNames({
      [`${prefixCls}-container`]: true,
      [`${prefixCls}-has-sider`]: nsTreeVisible,
    });

    return (
      <Layout className={layoutCls}>
        <Sider
          className={`${prefixCls}-sider-nstree`}
          width={nsTreeVisible ? 200 : 0}
        >
          <NsTree
            loading={this.state.treeLoading}
            treeData={this.state.treeData}
            originTreeData={this.state.originTreeData}
            expandedKeys={this.state.expandedKeys}
            onSearchValue={(val) => {
              this.setState({
                treeSearchValue: val,
              }, () => {
                this.fetchTreeData();
              });
            }}
            onExpandedKeys={(val) => {
              this.setState({ expandedKeys: val });
            }}
          />
        </Sider>
        <Content className={`${prefixCls}-content`}>
          <div className={`${prefixCls}-main`}>
            {this.props.children}
          </div>
        </Content>
      </Layout>
    );
  }

  render() {
    const { menuConf, language, onLanguageChange } = this.props;
    const { checkAuthenticateLoading, collapsed, selectedNode, nsTreeVisible } = this.state;
    const prefixCls = `${appname}-layout`;
    const { dispname, isroot } = auth.getSelftProfile();
    const logoSrc = collapsed ? require('../../assets/logo-s.png') : require('../../assets/logo-l.png');
    const userIconSrc = require('../../assets/favicon.ico');

    if (checkAuthenticateLoading) {
      return <div>Loading</div>;
    }

    return (
      <Layout className={prefixCls}>
        <Sider
          width={180}
          collapsedWidth={50}
          className={`${prefixCls}-sider-nav`}
          collapsible
          collapsed={collapsed}
          onCollapse={(newCollapsed) => {
            this.setState({ collapsed: newCollapsed }, () => {
              PubSub.publish('sider-collapse', true);
            });
          }}
        >
          <div
            className={`${prefixCls}-sider-logo`}
            style={{
              backgroundColor: '#353C46',
              height: 50,
              lineHeight: '50px',
              textAlign: 'center',
            }}
          >
            <img
              src={logoSrc}
              alt="logo"
              style={{
                height: 32,
              }}
            />
          </div>
          <LayoutMenu
            isroot={isroot}
            menuConf={menuConf}
            className={`${prefixCls}-menu`}
          />
        </Sider>
        <Layout>
          <Header className={`${prefixCls}-header`}>
            <div
              title={_.get(selectedNode, 'path')}
              style={{
                float: 'left',
                width: 400,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {nsTreeVisible ? _.get(selectedNode, 'path') : null}
            </div>
            <div className={`${prefixCls}-headRight`}>
              <Button
                style={{ margin: '0 20px' }}
                size="small"
                onClick={() => {
                  const newLanguage = language === 'zh' ? 'en' : 'zh';
                  onLanguageChange(newLanguage);
                }}
              >
                {language === 'zh' ? 'English' : ''}
                {language === 'en' ? '中文' : ''}
              </Button>
              <Dropdown placement="bottomRight" overlay={
                <Menu style={{ width: 110 }}>
                  <Menu.Item>
                    <Link to={{ pathname: '/profile' }}>
                      <Icon type="setting" className="mr10" />
                      {language === 'zh' ? '个人设置' : 'setting'}
                    </Link>
                  </Menu.Item>
                  <Menu.Item>
                    <a onClick={this.handleLogoutLinkClick}><Icon type="logout" className="mr10" />
                      {language === 'zh' ? '退出登录' : 'logout'}
                    </a>
                  </Menu.Item>
                </Menu>
              }>
                <span className={`${prefixCls}-username`}>
                  <span>Hi, {dispname}</span>
                  <img src={userIconSrc} alt="" />
                  <Icon type="down" />
                </span>
              </Dropdown>
            </div>
          </Header>
          <Content>
            {this.renderContent()}
          </Content>
        </Layout>
      </Layout>
    );
  }
}

export default injectIntl(withRouter(NILayout));
