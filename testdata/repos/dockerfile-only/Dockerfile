FROM alpine:3.19

RUN apk add --no-cache python3 py3-pip

WORKDIR /app

COPY app.py .

EXPOSE 8080

CMD ["python3", "app.py"]
