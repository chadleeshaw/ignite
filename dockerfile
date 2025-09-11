FROM alpine:latest
ENV DB_PATH=./
ENV DB_FILE=ignite.db
ENV DB_BUCKET=dhcp
ENV BIOS_FILE=boot-bios/pxelinux.0
ENV EFI_FILE=boot-efi/syslinux.efi
ENV TFTP_DIR=./public/tftp
ENV HTTP_DIR=./public/http
ENV HTTP_PORT=8080
ENV PROV_DIR=./public/provision
WORKDIR /app
RUN wget -O ./app https://github.com/chadleeshaw/ignite/releases/download/v2.0/app && \
    chmod +x ./app
COPY ./public /app/public
EXPOSE 8080
CMD ["./ignite"]
