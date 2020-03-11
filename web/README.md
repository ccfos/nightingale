# nightingale-web

## 开发

```shell
npm run start
```

访问 http://127.0.0.1:8000

## 构建

```shell
npm run build
```

构建后的代码默认会存放到 `/pub` 文件夹里

## Dll

```shell
npm run dll
```
webpack dllPlugin  
将相对稳定的包输出成 dll 减低生成环境的构建时间(开发环境不会依赖 dll)，如果这些包有更新需要构建新的 dll，默认的包请查看 `config/webpack.dll.config.js`

## 目录结构

- config: 开发 & 构建配置
  - proxy.config.js：代理配置
  - theme.js：antd 主题配置
  - webpack.config.js：webpack 开发环境补充配置，覆盖默认配置
  - webpack.build.config.js：webpack 构建补充配置，覆盖默认配置
  - webpack.build.config.js：webpack dll补充配置，覆盖默认配置
  - webpackConfigResolveAlias.js 文件路径别名配置
- src：源代码所在目录
  - assets：全局资源 img、css
  - common: 全局配置、通用方法
  - components：公共组件
  - pages：路由匹配的页面组件
  - app.jsx 菜单、路由配置组件
  - index.html：单页
  - index.jsx：入口文件
  - fetk.config.js 开发工具配置页面

