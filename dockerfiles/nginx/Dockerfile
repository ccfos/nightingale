FROM nginx
WORKDIR /home/n9e
COPY nginx.sh .
COPY nginx.conf /etc/nginx
ADD http://116.85.64.82/pub.20200928.tar.gz .
RUN tar xvf pub.20200928.tar.gz
