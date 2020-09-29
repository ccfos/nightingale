FROM centos:7
WORKDIR /home/n9e
COPY entrpoint.sh .
ADD http://116.85.64.82/n9e-3.0.0.tar.gz .
RUN tar xvf n9e-3.0.0.tar.gz
RUN yum install mysql net-tools -y
ENTRYPOINT ./entrpoint.sh
