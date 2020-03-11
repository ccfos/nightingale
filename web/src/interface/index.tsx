import React from 'react';

export interface RequestOption {
  method?: 'POST' | 'PUT' | 'DELETE',
  body?: string;
}

export interface MenuConfItem {
  key?: string,
  name: string | React.ReactNode,
  path: string,
  icon?: string,
  component?: React.ReactNode,
  children?: MenuConfItem[],
  visible?: boolean,
  rootVisible?: boolean,
  to?: string,
  divider?: boolean,
  target?: string,
  getQuery?: (query: any) => any,
}

export interface TreeNode {
  id: number,
  pid: number,
  name: string,
  path: string,
  type: number,
  leaf: number,
  cate?: string,
  children?: TreeNode[],
  icon_color: string,
  icon_char: string,
}

export interface ResponseDat {
  list: any[],
  total: number,
}

export interface Response {
  err : string,
  dat: any | ResponseDat,
}

export interface UserProfile {
  id: number,
  username: string,
  dispname: string,
  email: string,
  phone: string,
  im: string,
  isroot: boolean,
  is_root: 0 | 1,
}

export interface Team {
  id: number,
  ident: string,
  name: string,
  note: string,
  mgmt: number,
  admin_objs: UserProfile[],
  member_objs: UserProfile[],
  admins?: string[],
  members?: string[],
}

export interface Endpoint {
  id: number,
  ident: string,
  alias: string,
  nodes?: string[],
}
