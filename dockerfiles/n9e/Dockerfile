FROM centos:7
WORKDIR /home/n9e

RUN set -ex \
    && yum install mysql net-tools wget -y \
    && yum clean all \
    && rm -rf /var/cache/yum

ADD http://116.85.64.82/n9e.tar.gz .

RUN tar xvf n9e.tar.gz \
    && sed -i "s/127.0.0.1:6379/redis:6379/g" `grep -rl "127.0.0.1:6379" ./etc/` \
    && sed -i 's/127.0.0.1/mysql/g' etc/mysql.yml

COPY entrpoint.sh .

ENTRYPOINT ./entrpoint.sh
