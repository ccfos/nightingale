import React, { Component } from 'react';
import { Table } from 'antd';
import _ from 'lodash';
import queryString from 'query-string';
import api from '@common/api';
import * as config from '@common/config';
import request from '@common/request';
import './style.less';

export default class BaseComponent extends Component {
  constructor(props) {
    super(props);
    this.api = api;
    this.config = config;
    this.prefixCls = config.appname;
    this.request = request;
    this.otherParamsKey = [];
    this.state = {
      loading: false,
      pagination: {
        current: 1,
        pageSize: 10,
        showSizeChanger: true,
      },
      data: [],
      searchValue: '',
    };
  }

  handleSearchChange = (value) => {
    this.setState({ searchValue: value }, () => {
      this.reload({
        query: value,
      }, true);
    });
  }

  handleTableChange = (pagination) => {
    const { pagination: paginationState } = this.state;
    const pager = {
      ...paginationState,
      current: pagination.current,
      pageSize: pagination.pageSize,
    };
    this.setState({ pagination: pager }, () => {
      this.reload({
        limit: pagination.pageSize,
        page: pagination.current,
      });
    });
  }

  reload(params) {
    this.fetchData(params);
  }

  fetchData(newParams = {}, backFirstPage = false) {
    const url = this.getFetchDataUrl();

    if (!url) return;
    const othenParams = _.pick(this.state, this.otherParamsKey);
    const { pagination, searchValue } = this.state;
    const params = {
      limit: pagination.pageSize,
      p: backFirstPage ? 1 : pagination.current,
      query: searchValue,
      ...othenParams,
      ...newParams,
    };

    this.setState({ loading: true });
    // TODO: Method 'fetchData' expected no return value.
    // eslint-disable-next-line consistent-return
    return this.request(`${url}?${queryString(params)}`).then((res) => {
      const newPagination = {
        ...pagination,
        current: backFirstPage ? 1 : pagination.current,
        total: res.total,
      };
      let data = [];
      if (_.isArray(res.list)) {
        data = res.list;
      } else if (_.isArray(res)) {
        data = res;
      }
      this.setState({
        data,
        pagination: newPagination,
      });
      return data;
    }).finally(() => {
      this.setState({ loading: false });
    });
  }

  renderTable(params) {
    const { loading, pagination, data } = this.state;
    return (
      <Table
        rowKey="id"
        size="small"
        loading={loading}
        pagination={{
          ...pagination,
          showTotal: total => `共 ${total} 条数据`,
          pageSizeOptions: config.defaultPageSizeOptions,
          onChange: () => {
            if (this.handlePaginationChange) this.handlePaginationChange();
          },
        }}
        rowClassName={(record, index) => {
          if (index % 2 === 1) {
            return 'table-row-bg';
          }
          return '';
        }}
        dataSource={data}
        onChange={this.handleTableChange}
        {...params}
      />
    );
  }
}
