FROM golang as builder
WORKDIR /home/n9e/builder
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/
RUN git clone  https://hub.fastgit.org/didi/nightingale.git
RUN nightingale/control build

FROM centos:7
WORKDIR /home/n9e
COPY --from=builder /home/n9e/builder/nightingale/ .
COPY entrpoint.sh .
RUN yum install mysql net-tools -y
ENTRYPOINT ./entrpoint.sh