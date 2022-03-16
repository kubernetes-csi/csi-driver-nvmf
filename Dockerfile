FROM debian:9.13

COPY ./bin/nvmfplugin .

ENTRYPOINT ["/nvmfplugin"]
