## sql 的维护

- n9e_{module}.sql 完整的sql
- n9e_{module}-path.sql 增量的sql

## sql 的版本发布
在使用 git tag 之前，将特定版本的增量文件固化下来

```
module=rdb
version=v3.3.3
cat n9e_${module}-patch.sql > upgrade/n9e_${module}-${version}.sql
echo > n9e_${module}-patch.sql

# 然后提交更改后，再打上版本的tag
git add .
git commit -a -m "${version} release"
git tag ${version}
git push
```
