FROM debian:9.13

RUN apt-get update && apt-get install -y nvme-cli

COPY ./bin/nvmfplugin .

ENTRYPOINT ["/nvmfplugin"]
