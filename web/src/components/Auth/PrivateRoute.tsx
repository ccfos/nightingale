import React from 'react';
import { Route, Redirect, RouteProps } from 'react-router-dom';
import auth from './auth';

interface Props extends RouteProps{
  component: typeof React.Component,
  rootVisible?: boolean,
}

export default function PrivateRoute({ component: Component, rootVisible = false, ...rest }: Props) {
  const { isroot } = auth.getSelftProfile();
  const isAuthenticated = auth.getIsAuthenticated();
  return (
    <Route
      {...rest}
      render={(props) => {
        if (isAuthenticated) {
          if (rootVisible && !isroot) {
            return (
              <Redirect
                to={{
                  pathname: '/403',
                }}
              />
            );
          }
          return <Component {...props} />;
        }
        return (
          <Redirect
            to={{
              pathname: '/login',
              state: { from: props.location }, // eslint-disable-line react/prop-types
            }}
          />
        );
      }}
    />
  );
}
