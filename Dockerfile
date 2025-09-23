FROM debian:13

RUN apt-get update && apt-get install -y e2fsprogs && apt-get clean all
COPY ./bin/nvmfplugin .

ENTRYPOINT ["/nvmfplugin"]
