FROM haproxy:1.8.2-alpine

COPY bin/kube-bmlb /bin
EXPOSE 80 9010
CMD ["kube-bmlb", "--logtostderr"]