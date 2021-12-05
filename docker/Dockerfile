FROM ubuntu:21.04

WORKDIR /app
ADD n9e /app
RUN mkdir -p /app/pub
ADD pub /app/pub/
RUN chmod +x n9e

EXPOSE 19000
EXPOSE 18000

CMD ["/app/n9e", "-h"]