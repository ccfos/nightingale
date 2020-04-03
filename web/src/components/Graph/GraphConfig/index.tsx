import React, { Component } from 'react';
import { FormattedMessage } from 'react-intl';
import PropTypes from 'prop-types';
import { Modal, Button, message } from 'antd';
import _ from 'lodash';
import GraphConfigForm from './GraphConfigForm';
import './style.less';

/**
 * graph 配置面板组件
 */

export default class GraphConfig extends Component {
  static propTypes = {
    onChange: PropTypes.func.isRequired,
  };

  static defaultProps = {
  };

  constructor(props) {
    super(props);
    this.state = {
      key: _.uniqueId('graphConfigModal_'),
      visible: false,
      title: '图表配置',
      type: 'add',
      btnName: '看图',
      btnDisabled: false,
      data: {}, // graphConfig
      isScreen: false,
      subclassOptions: [],
    };
  }

  showModal(
    type = this.state.type,
    btnName = this.state.btnName,
    data = {},
  ) {
    const { isScreen, subclassOptions } = data;
    delete data.isScreen;
    delete data.subclassOptions;

    this.setState({
      key: _.uniqueId('graphConfigModal_'),
      visible: true,
      type,
      btnName,
      data,
      isScreen,
      subclassOptions,
    });
  }

  handleSubmit(type, id) {
    // eslint-disable-next-line react/no-string-refs
    const { graphConfigForm } = this.refs;
    const { onChange } = this.props;
    const formState = graphConfigForm.state.graphConfig;
    const { start, end } = formState;

    if (Number(start) > Number(end)) {
      message.error('开始时间不能大于结束时间');
      return;
    }

    this.setState({
      visible: false,
    }, () => {
      onChange(type, {
        ...formState,
      }, id);
    });
  }

  renderFooter() {
    const { type, data, btnName, btnDisabled } = this.state;

    if (type === 'push' || type === 'unshift') {
      return (
        <Button
          type="primary"
          disabled={btnDisabled}
          onClick={() => {
            this.handleSubmit(type);
          }}
        >
          {btnName}
        </Button>
      );
    }
    if (type === 'update') {
      return (
        <Button
          key="submit"
          type="primary"
          disabled={btnDisabled}
          onClick={() => {
            this.handleSubmit(type, data.id);
          }}
        >
          {btnName}
        </Button>
      );
    }
    return null;
  }

  render() {
    const { key, title, visible, data, isScreen, subclassOptions } = this.state;

    return (
      <Modal
        key={key}
        width={750}
        title={<FormattedMessage id="graph.config.title" />}
        destroyOnClose
        visible={visible}
        maskClosable={false}
        wrapClassName="ant-modal-GraphConfig"
        footer={this.renderFooter()}
        onCancel={() => {
          this.setState({ visible: false });
        }}
      >
        <div className="graph-config-form-container">
          <GraphConfigForm
            // eslint-disable-next-line react/no-string-refs
            ref="graphConfigForm"
            data={data}
            isScreen={isScreen}
            subclassOptions={subclassOptions}
            btnDisable={(disabled) => {
              this.setState({
                btnDisabled: disabled,
              });
            }}
          />
        </div>
      </Modal>
    );
  }
}
