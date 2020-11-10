FROM golang as builder
WORKDIR /home/n9e/builder
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/
RUN git clone  https://hub.fastgit.org/didi/nightingale.git
RUN nightingale/control build

FROM centos:7
WORKDIR /home/n9e
COPY --from=builder /home/n9e/builder/nightingale/ .
COPY entrpoint.sh .
RUN sed -i 's/127.0.0.1:6379/redis:6379/g' etc/judge.yml
RUN yum install mysql net-tools -y
ENTRYPOINT ./entrpoint.sh
