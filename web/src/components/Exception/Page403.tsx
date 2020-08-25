import React from 'react';
import { RouteComponentProps } from 'react-router-dom';
import { Button } from 'antd';
import { appname } from '@common/config';

export default function Page403({ history }: RouteComponentProps) {
  const prefixCls = `${appname}-exception`;
  return (
    <div className={prefixCls}>
      <div className={`${prefixCls}-main`}>
        <div className={`${prefixCls}-title`}>403</div>
        <div className={`${prefixCls}-content mb10`}>抱歉，你无权访问该页面</div>
        <Button
          icon="arrow-left"
          type="primary"
          onClick={() => {
            history.push({
              pathname: '/',
            });
          }}
        >
          返回首页
        </Button>
      </div>
    </div>
  );
}
