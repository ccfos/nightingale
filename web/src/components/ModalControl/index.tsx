import React from 'react';
import ReactDOM from 'react-dom';

export default function ModalControlWrap(Component: typeof React.Component) {
  return function ModalControl(config: any) {
    const div = document.createElement('div');
    document.body.appendChild(div);

    function destroy() {
      const unmountResult = ReactDOM.unmountComponentAtNode(div);
      if (unmountResult && div.parentNode) {
        div.parentNode.removeChild(div);
      }
    }

    function render(props: any) {
      ReactDOM.render(<Component {...props} />, div);
    }

    render({ ...config, visible: true, destroy });

    return {
      destroy,
    };
  };
}
