FROM python:3-slim
#FROM ubuntu:21.04

WORKDIR /app
ADD n9e /app
ADD http://download.flashcat.cloud/wait /wait
RUN mkdir -p /app/pub && chmod +x /wait
ADD pub /app/pub/
RUN chmod +x n9e

EXPOSE 17000

CMD ["/app/n9e", "-h"]
