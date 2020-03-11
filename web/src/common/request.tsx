import React, { Component } from 'react';
import { Icon, Progress } from 'antd';
import Notification from 'rc-notification';
import 'rc-notification/assets/index.css';
import _ from 'lodash';
import * as config from '@common/config';
import { Response, RequestOption } from '@interface';

const controller = new AbortController();
const { signal } = controller;
let notification: any;
Notification.newInstance({
  style: {
    top: 24,
    right: 0,
    zIndex: 1001,
  },
}, (n: any) => { notification = n; });

/**
 * 后端接口非 5xx 都会返回 2xx
 * 异常都是通过 res.err 来判断，res.err 有值则请求失败。res.err 是具体的错误信息
 * res.err 为 'unauthorized' 约定的未授权状态
*/

interface Props {
  duration: number,
  msg: string,
  onClose: () => void,
}

class ErrNotifyContent extends Component<Props> {
  timerId = 0;

  state = {
    percent: 0,
  };

  componentDidMount = () => {
    this.setUpTimer();
  }

  componentWillUnmount() {
    if (this.timerId) {
      window.clearInterval(this.timerId);
    }
  }

  setUpTimer() {
    const { duration, onClose } = this.props;
    let { percent } = this.state;
    this.timerId = window.setInterval(() => {
      if (percent < 100) {
        percent += 10 / duration;
        this.setState({ percent });
      } else {
        window.clearInterval(this.timerId);
        onClose();
      }
    }, 100);
  }

  render() {
    return (
      <div
        style={{
          width: 350,
          padding: '16px 24px',
        }}
        onMouseOver={() => {
          if (this.timerId) {
            window.clearInterval(this.timerId);
            this.setState({ percent: 0 });
          }
        }}
        onMouseOut={() => {
          this.setUpTimer();
        }}
        onFocus={() => {}}
        onBlur={() => {}}
      >
        <Progress
          className={`${config.appname}-errNotify-progress`}
          percent={this.state.percent}
          showInfo={false}
          style={{
            position: 'absolute',
            bottom: 0,
            left: 0,
            opacity: 0.2,
          }}
        />
        <Icon
          type="close-circle"
          style={{
            color: '#f5222d',
            fontSize: 24,
          }}
        />
        <div
          style={{
            display: 'inline-block',
            fontSize: 16,
            lineHeight: '24px',
            verticalAlign: 'top',
            marginLeft: 10,
          }}
        >
          请求错误
        </div>
        <div
          style={{
            marginLeft: 35,
          }}
        >
          {this.props.msg}
        </div>
      </div>
    );
  }
}

function errNotify(errMsg: string) {
  const notifyId = _.uniqueId('notifyId_');

  notification.notice({
    key: notifyId,
    duration: 0,
    closable: true,
    style: {
      right: '20px',
    },
    content: (
      <ErrNotifyContent
        msg={errMsg}
        duration={5}
        onClose={() => {
          notification.removeNotice(notifyId);
        }}
      />
    ),
  });
}

export default async function request(url: string, options?: RequestOption, isUseDefaultErrNotify = true) {
  const response = await fetch(url, {
    headers: {
      'content-type': 'application/json',
    },
    ...options,
    signal,
  });

  if (
    response.status < 200
    || response.status >= 300
  ) {
    errNotify(response.statusText);
    const error = new Error(response.statusText);
    throw error;
  }

  const data: Response = await response.json();

  if (typeof data === 'object' && data.err !== '') {
    if (data.err === 'unauthorized') {
      window.location.href = config.loginPath;
      throw 'unauthorized';
    } else {
      if (isUseDefaultErrNotify) errNotify(data.err);
      const error = new Error(data.err);
      throw error;
    }
  }

  return data.dat;
}
