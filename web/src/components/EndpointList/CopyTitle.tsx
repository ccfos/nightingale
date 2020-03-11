import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Dropdown, Menu, Modal, Input, Icon, message } from 'antd';
import _ from 'lodash';
import clipboard from '@common/clipboard';
import request from '@common/request';
import api from '@common/api';

interface Props {
  type?: 'mgmt';
  data: any[];
  selected: any[];
  dataIndex: string;
  hasSelected: boolean;
}

export default class CopyTitle extends Component<Props> {
  static contextTypes = {
    getSelectedNode: PropTypes.func,
  };

  static propTypes = {
    data: PropTypes.array,
    selected: PropTypes.array,
    dataIndex: PropTypes.string.isRequired,
    hasSelected: PropTypes.bool,
  };

  static defaultProps = {
    data: [],
    selected: [],
    hasSelected: true,
  };

  handleCopyBtnClick = async (dataIndex: string, copyType: string) => {
    const { getSelectedNode } = this.context;
    const { data, selected } = this.props;
    let tobeCopy = [];

    if (copyType === 'all') {
      let allData = [];
      if (this.props.type === 'mgmt') {
        allData = await request(`${api.endpoint}?limit=100000`);
        allData = allData.list;
      } else {
        allData = await request(`${api.endpoint}s/bynodeids?ids=${getSelectedNode('id')}`);
      }
      tobeCopy = _.map(allData, item => item[dataIndex]);
    } else if (copyType === 'currentPage') {
      tobeCopy = _.map(data, item => item[dataIndex]);
    } else if (copyType === 'selected') {
      tobeCopy = _.map(selected, item => item[dataIndex]);
    }

    if (_.isEmpty(tobeCopy)) {
      message.warning('复制的对象为空');
      return;
    }

    const tobeCopyStr = _.join(tobeCopy, '\n');
    const copySucceeded = clipboard(tobeCopyStr);

    if (copySucceeded) {
      message.success(`复制成功${tobeCopy.length}条记录！`);
    } else {
      Modal.warning({
        title: '复制失败，请手动复制',
        content: <Input.TextArea defaultValue={tobeCopyStr} />,
      });
    }
  }

  render() {
    const { dataIndex, hasSelected } = this.props;
    const title = '';

    if (hasSelected) {
      return (
        <Dropdown
          trigger={['click']}
          overlay={
            <Menu>
              <Menu.Item>
                <a onClick={() => this.handleCopyBtnClick(dataIndex, 'selected')}>复制已选</a>
              </Menu.Item>
              <Menu.Item>
                <a onClick={() => this.handleCopyBtnClick(dataIndex, 'currentPage')}>复制当前页</a>
              </Menu.Item>
              <Menu.Item>
                <a onClick={() => this.handleCopyBtnClick(dataIndex, 'all')}>复制所有</a>
              </Menu.Item>
            </Menu>
          }
        >
          <span>
            {
              this.props.children ? this.props.children : title
            }
            <Icon type="copy" className="pointer" style={{ paddingLeft: 5 }} />
          </span>
        </Dropdown>
      );
    }
    return (
      <span>
        {
          this.props.children ? this.props.children : title
        }
        <Icon type="copy" className="pointer" style={{ paddingLeft: 5 }}
          onClick={() => this.handleCopyBtnClick(dataIndex, 'all')} />
      </span>
    );
  }
}
