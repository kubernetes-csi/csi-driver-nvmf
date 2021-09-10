FROM debian:9.13

COPY ./output/nvmfplugin .

ENTRYPOINT ["./nvmfplugin"]