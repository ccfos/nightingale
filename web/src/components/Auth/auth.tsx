import _ from 'lodash';
import api from '@common/api';
import request from '@common/request';
import { UserProfile } from '@interface';

export default (function auth() {
  let isAuthenticated = false;
  let selftProfile = {} as UserProfile;
  return {
    getIsAuthenticated() {
      return isAuthenticated;
    },
    getSelftProfile() {
      return selftProfile;
    },
    checkAuthenticate() {
      return request(api.selftProfile).then((res: any) => {
        isAuthenticated = true;
        selftProfile = {
          ...res,
          isroot: res.is_root === 1,
        };
      });
    },
    authenticate: async (reqBody: any, cbk: () => void) => {
      try {
        await request(api.login, {
          method: 'POST',
          body: JSON.stringify(reqBody),
        });
        isAuthenticated = true;
        selftProfile = await request(api.selftProfile);
        if (_.isFunction(cbk)) cbk(selftProfile);
      } catch (e) {
        console.log(e);
      }
    },
    signout(cbk: () => void) {
      request(api.logout).then((res) => {
        isAuthenticated = false;
        if (_.isFunction(cbk)) cbk(res);
      });
    },
  };
}());
