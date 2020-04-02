import React, { Component } from 'react';
import { injectIntl, WrappedComponentProps, FormattedMessage } from 'react-intl';
import { Row, Col, Input, Button, Divider, Popover, Popconfirm, message, Tooltip, Alert } from 'antd';
import { ColumnProps } from 'antd/lib/table';
import { UserProfile } from '@interface';
import clipboard from '@common/clipboard';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import { auth } from '@cpts/Auth';
import request from '@common/request';
import api from '@common/api';
import FetchTable from '@cpts/FetchTable';
import CreateUser from './CreateUser';
import PutPassword from './PutPassword';
import PutProfile from './PutProfile';

interface State {
  searchValue: string,
  inviteTooltipVisible: boolean,
  invitePopoverVisible: boolean,
  inviteLink: string,
  copySucceeded: boolean,
}

const ButtonGroup = Button.Group;

class User extends Component<WrappedComponentProps, State> {
  fetchtable: any;
  state = {
    inviteTooltipVisible: false,
    invitePopoverVisible: false,
    inviteLink: '',
    copySucceeded: false,
  } as State;

  handleInviteBtnClick = () => {
    request(`${api.users}/invite`).then((res) => {
      const { origin, pathname } = window.location;
      const inviteLink = `${origin}${pathname}#/register?token=${res}`;
      const copySucceeded = clipboard(inviteLink);

      this.setState({
        copySucceeded,
        inviteLink,
        inviteTooltipVisible: false,
        invitePopoverVisible: true,
      });
    });
  }

  handleAddBtnClick = () => {
    CreateUser({
      title: this.props.intl.formatMessage({ id: 'user.create' }),
      language: this.props.intl.locale,
      onOk: () => {
        this.fetchtable.reload();
      },
    });
  }

  handlePutBtnClick = (record: UserProfile) => {
    PutProfile({
      title: this.props.intl.formatMessage({ id: 'user.modify' }),
      language: this.props.intl.locale,
      data: record,
      onOk: () => {
        this.fetchtable.reload();
      },
    });
  }

  handlePutPassBtnClick = (id: number) => {
    PutPassword({
      title: this.props.intl.formatMessage({ id: 'user.reset.password' }),
      language: this.props.intl.locale,
      id,
      onOk: () => {
        this.fetchtable.reload();
      },
    });
  }

  handleDelBtnClick(id: number) {
    request(`${api.user}/${id}`, {
      method: 'DELETE',
    }).then(() => {
      this.fetchtable.reload();
      message.success(this.props.intl.formatMessage({ id: 'msg.delete.success' }));
    });
  }

  render() {
    const {
      invitePopoverVisible,
      inviteTooltipVisible,
      copySucceeded,
      inviteLink,
    } = this.state;
    const { isroot } = auth.getSelftProfile();
    const columns: ColumnProps<UserProfile>[] = [
      {
        title: <FormattedMessage id="user.username" />,
        dataIndex: 'username',
      }, {
        title: <FormattedMessage id="user.dispname" />,
        dataIndex: 'dispname',
      }, {
        title: <FormattedMessage id="user.email" />,
        dataIndex: 'email',
      }, {
        title: <FormattedMessage id="user.phone" />,
        dataIndex: 'phone',
      }, {
        title: 'im',
        dataIndex: 'im',
      }, {
        title: <FormattedMessage id="user.isroot" />,
        dataIndex: 'is_root',
        width: 70,
        className: 'textAlignCenter',
        render: (text) => {
          if (this.props.intl.locale === 'zh') {
            return text === 1 ? '是' : '否';
          }
          return text === 1 ? 'Y' : 'N';
        },
      },
    ];
    if (isroot) {
      columns.push({
        title: <FormattedMessage id="table.operations" />,
        className: 'textAlignCenter',
        width: this.props.intl.locale === 'zh' ? 200 : 250,
        render: (_text, record) => {
          return (
            <span>
              <a onClick={() => { this.handlePutPassBtnClick(record.id); }}><FormattedMessage id="user.reset.password" /></a>
              <Divider type="vertical" />
              <a onClick={() => { this.handlePutBtnClick(record); }}><FormattedMessage id="table.modify" /></a>
              <Divider type="vertical" />
              <Popconfirm title={<FormattedMessage id="table.delete.sure" />} onConfirm={() => { this.handleDelBtnClick(record.id); }}>
                <a><FormattedMessage id="table.delete" /></a>
              </Popconfirm>
            </span>
          );
        },
      });
    }
    return (
      <div>
        <Row>
          <Col span={8} className="mb10">
            <Input.Search
              style={{ width: 200 }}
              onSearch={(val) => {
                this.setState({ searchValue: val });
              }}
            />
          </Col>
          <Col span={16} className="textAlignRight">
            <ButtonGroup>
              {
                isroot ? <Button onClick={this.handleAddBtnClick}><FormattedMessage id="user.create" /></Button> : null
              }
              <Popover
                trigger="click"
                placement="topRight"
                visible={invitePopoverVisible}
                onVisibleChange={(visible) => {
                  if (!visible) {
                    this.setState({ invitePopoverVisible: visible });
                  }
                }}
                content={
                  copySucceeded ?
                    <Alert message={<FormattedMessage id="invite.user.copy.success" />} type="success" /> :
                    <Alert message={
                      <div>
                        <p><FormattedMessage id="invite.user.copy.faile" /></p>
                        <span>{inviteLink}</span>
                      </div>
                    } type="warning" />
                }
              >
                <Tooltip
                  placement="topRight"
                  visible={inviteTooltipVisible}
                  onVisibleChange={(visible) => { this.setState({ inviteTooltipVisible: visible }); }}
                  title={<FormattedMessage id="user.invite.tips" />}
                >
                  <Button className="ml10" onClick={this.handleInviteBtnClick}><FormattedMessage id="user.invite" /></Button>
                </Tooltip>
              </Popover>
            </ButtonGroup>
          </Col>
        </Row>
        <FetchTable
          ref={(ref) => { this.fetchtable = ref; }}
          backendPagingEnabled={true}
          url={api.user}
          query={{ query: this.state.searchValue }}
          tableProps={{
            columns,
          }}
        />
      </div>
    );
  }
}
export default CreateIncludeNsTree(injectIntl(User));
