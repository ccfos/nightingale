FROM golang as builder
WORKDIR /home/n9e/builder
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/
RUN git clone  https://hub.fastgit.org/didi/nightingale.git
RUN nightingale/control build

FROM centos:7
WORKDIR /home/n9e
COPY --from=builder /home/n9e/builder/nightingale/ .
RUN \
sed -i "s/127.0.0.1:6379/redis:6379/g" `grep -rl "127.0.0.1:6379" ./etc/` &&\
sed -i 's/127.0.0.1/mysql/g' etc/mysql.yml
COPY entrpoint.sh .
RUN set -ex \
    && yum install mysql net-tools -y \
    && yum clean all \
    && rm -rf /var/cache/yum
ENTRYPOINT ./entrpoint.sh
