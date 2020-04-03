import * as React from 'react';
import { useContext } from 'react';
import { injectIntl } from 'react-intl';

export const IntlContext = React.createContext({} as any);

// turn the old context into the new context
export const InjectIntlContext = injectIntl(({ intl, children }) => (
  <IntlContext.Provider value={intl}>
    { children }
  </IntlContext.Provider>
));

export const getIntl = () => useContext(IntlContext);

// the format message hook
const useFormatMessage = () => {
  const intl = useContext(IntlContext);
  return intl.formatMessage;
};

export default useFormatMessage;
