import React, { Component } from 'react';
import { Link, matchPath, withRouter, RouteComponentProps, LinkProps } from 'react-router-dom';
import queryString from 'query-string';
import { Menu, Icon } from 'antd';
import _ from 'lodash';
import { MenuConfItem } from '@interface';
import * as utils from './utils';

interface Props {
  className?: string,
  defaultOpenAllNavs?: boolean,
  isroot: boolean,
  menuMode?: 'inline' | 'vertical' | 'vertical-left' | 'vertical-right' | 'horizontal' | undefined,
  menuTheme?: 'dark' | 'light' | undefined,
  menuStyle?: any,
  menuConf: MenuConfItem[],
}

const { Item: MenuItem, Divider: MenuDivider, SubMenu } = Menu;

class LayoutMenu extends Component<Props & RouteComponentProps> {
  static defaultProps = {
    menuMode: 'inline',
    menuTheme: 'dark',
    menuStyle: undefined,
  } as Props;

  defaultOpenKeys: string[] = [];
  selectedKeys: string[] = [];

  componentWillReceiveProps() {
    this.selectedKeys = [];
  }

  getNavMenuItems(navs: MenuConfItem[]) {
    const { location, menuMode, defaultOpenAllNavs } = this.props;

    return _.map(_.filter(navs, (nav) => {
      if (!this.props.isroot && nav.rootVisible) {
        return false;
      }
      return true;
    }), (nav, index) => {
      if (nav.divider) {
        return <MenuDivider key={index} />;
      }

      const icon = nav.icon ? <Icon className={`Linear ${nav.icon}`} type={nav.icon} /> : null;
      const linkProps = {} as LinkProps;
      let link;

      if (_.isArray(nav.children) && utils.hasRealChildren(nav.children)) {
        const menuKey = nav.key || nav.to;

        if (defaultOpenAllNavs) {
          if (menuKey) this.defaultOpenKeys.push(menuKey);
        } else if (nav.to && this.isActive(nav.to) && menuMode === 'inline') {
          this.defaultOpenKeys = _.union(this.defaultOpenKeys, [nav.to]);
        }

        return (
          <SubMenu
            key={menuKey}
            title={
              <span>
                {icon}
                <span>{nav.name}</span>
              </span>
            }
          >
            {this.getNavMenuItems(nav.children)}
          </SubMenu>
        );
      }

      if (nav.target) {
        linkProps.target = nav.target;
      }

      if (nav.to && utils.isAbsolutePath(nav.to)) {
        linkProps.href = nav.to;
        link = (
          <a {...linkProps}>
            {icon}
            <span>{nav.name}</span>
          </a>
        );
      } else {
        if (nav.to && this.isActive(nav.to)) this.selectedKeys = [nav.to];

        linkProps.to = {
          pathname: nav.to,
        };

        if (_.isFunction(nav.getQuery)) {
          const query = nav.getQuery(queryString.parse(location.search));
          linkProps.to.search = queryString.stringify(query);
        }

        link = (
          <Link to={linkProps.to}>
            {icon}
            <span>{nav.name}</span>
          </Link>
        );
      }

      return (
        <MenuItem
          key={nav.to}
        >
          {link}
        </MenuItem>
      );
    });
  }

  isActive(path: string) {
    const { location } = this.props;
    return !!matchPath(location.pathname, { path });
  }

  render() {
    const {
      menuMode,
      menuTheme,
      menuStyle,
      location,
    } = this.props;
    const { menuConf, className } = this.props;
    const realMenuConf = _.isFunction(menuConf) ? menuConf(location) : menuConf;
    const normalizedMenuConf = utils.normalizeMenuConf(realMenuConf);
    const menus = this.getNavMenuItems(normalizedMenuConf);

    return (
      <Menu
        defaultOpenKeys={this.defaultOpenKeys}
        selectedKeys={this.selectedKeys}
        theme={menuTheme}
        mode={menuMode}
        style={menuStyle}
        className={className}
      >
        {menus}
      </Menu>
    );
  }
}

export default withRouter(LayoutMenu);
