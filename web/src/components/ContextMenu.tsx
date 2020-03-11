import React, { Component } from 'react';
import _ from 'lodash';

interface Props {
  children: React.ReactNode,
  visible: boolean,
  top: number,
  left: number,
  onVisibleChang: () => void,
}

interface State {
  visible: boolean,
  top: number,
  left: number,
}

export default class ContextMenu extends Component<Props, State> {
  static defaultProps = {
    visible: false,
    top: 0,
    left: 0,
    onVisibleChang: _.noop,
  };

  constructor(props: Props) {
    super(props);
    const { visible, top, left } = props;
    this.state = {
      visible, top, left,
    };
  }

  componentDidMount() {
    document.addEventListener('click', this.handleDocumentContextMenuClick);
  }

  componentWillReceiveProps(nextProps: Props) {
    const { visible, top, left } = nextProps;
    this.setState({
      visible, top, left,
    });
  }

  componentWillUnmount() {
    document.removeEventListener('click', this.handleDocumentContextMenuClick);
  }

  handleDocumentContextMenuClick = () => {
    if (this.state.visible) {
      this.setState({
        visible: false,
      }, () => {
        if (_.isFunction(this.props.onVisibleChang)) this.props.onVisibleChang(false);
      });
    }
  }

  render() {
    const { top, left, visible } = this.state;
    return (
      <div style={{
        display: visible ? 'block' : 'none',
        position: 'fixed',
        top,
        left,
      }}>
        {this.props.children}
      </div>
    );
  }
}
