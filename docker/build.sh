#!/bin/sh
if [ $# -ne 1 ]; then
	echo "$0 <tag>"
	exit 0
fi

tag=$1

echo "tag: ${tag}"

rm -rf n9e pub
cp ../n9e .

docker build -t nightingale:${tag} .

docker tag nightingale:${tag} ulric2019/nightingale:${tag}
docker push ulric2019/nightingale:${tag}

rm -rf n9e pub
