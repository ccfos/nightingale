FROM golang:1.13

LABEL maintainer="llitfkitfk@gmail.com,chenjiandongx@qq.com"

WORKDIR /app

RUN apt-get update && apt-get install net-tools -y

COPY . .
RUN ./control build docker
RUN mv /app/bin/* /usr/local/bin
