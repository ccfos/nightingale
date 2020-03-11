import React, { Component } from 'react';
import { Tabs, Button, message } from 'antd';
import auth from '@cpts/Auth/auth';
import ProfileForm from '@cpts/ProfileForm';
import CreateIncludeNsTree from '@cpts/Layout/CreateIncludeNsTree';
import request from '@common/request';
import api from '@common/api';
import { appname } from '@common/config';
import PutPasswordForm from './PutPasswordForm';

const { TabPane } = Tabs;

class index extends Component {
  profileFormRef: any;
  putPasswordFormRef: any;
  handlePutProfileSubmit = () => {
    this.profileFormRef.validateFields((err: any, values: any) => {
      if (!err) {
        request(api.selftProfile, {
          method: 'PUT',
          body: JSON.stringify(values),
        }).then(() => {
          message.success('信息修改成功！');
        });
      }
    });
  }

  handlePutPasswordSubmit = () => {
    this.putPasswordFormRef.validateFields((err: any, values: any) => {
      if (!err) {
        request(api.selftPassword, {
          method: 'PUT',
          body: JSON.stringify(values),
        }).then(() => {
          message.success('密码修改成功！');
        });
      }
    });
  }

  render() {
    const prefixCls = `${appname}-profile`;
    const profile = auth.getSelftProfile();
    return (
      <div className={prefixCls}>
        <Tabs tabPosition="left">
          <TabPane tab="基础设置" key="baseSetting">
            <div style={{ width: 500 }}>
              <ProfileForm type="put" initialValue={profile} ref={(ref) => { this.profileFormRef = ref; }} />
              <Button type="primary" onClick={this.handlePutProfileSubmit}>提交</Button>
            </div>
          </TabPane>
          <TabPane tab="修改密码" key="resetPassword">
            <div style={{ width: 500 }}>
              <PutPasswordForm ref={(ref: any) => { this.putPasswordFormRef = ref; }} />
              <Button type="primary" onClick={this.handlePutPasswordSubmit}>提交</Button>
            </div>
          </TabPane>
        </Tabs>
      </div>
    );
  }
}
export default CreateIncludeNsTree(index);
