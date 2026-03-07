apiVersion: drivers/v1
kind: Driver
metadata:
  name: test
spec:
  driver: postgresql
  config:
    dsn: postgres://test:test@localhost:5432/test
