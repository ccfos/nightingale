import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';

export default function CreateIncludeNsTree(WrappedComponent: any, opts?: any) {
  return class HOC extends React.Component {
    static contextTypes = {
      nsTreeVisibleChange: PropTypes.func,
    };

    componentWillMount() {
      const { nsTreeVisibleChange } = this.context;
      nsTreeVisibleChange(_.get(opts, 'visible', false));
    }

    render() {
      return <WrappedComponent {...this.props} />;
    }
  };
}
