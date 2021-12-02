# e-Distribucion API

Go API to read Endesa energy meters info.

You'll need an account in https://www.edistribucion.com to use this module.

## Building the command line client

```
go build -o contadores ./cmd/edistribucion/main.go
```

## Using the client

Export the edistribicion.com username and password as environment variables:

```
export EDISTRIBUCION_USERNAME="username here"
export EDISTRIBUCION_PASSWORD="password here"
```

Run the client without arguments:

```
./contadores
<Your CUPS address will be here>
Potencia actual:  0.2
Potencia contratada:  5.75
Porcentage:  3,48%
Estado ICP:  Abierto
Totalizador:  14.811
```