FROM nginx
WORKDIR /home/n9e
COPY nginx.conf /etc/nginx
ADD https://github.com/n9e/fe/releases/download/3.5.6/pub-3.5.6.zip .
RUN apt update && apt install unzip
RUN unzip pub-3.5.6.zip
