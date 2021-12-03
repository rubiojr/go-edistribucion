# e-Distribucion API

Go API to read Endesa energy meters info.

Ported from Python to Go using https://github.com/trocotronic/edistribucion.

You'll need an account in https://www.edistribucion.com to use this module.

## Building the command line client

```
make
```

## Using the client

Export the edistribicion.com username and password as environment variables:

```
export EDISTRIBUCION_USERNAME="username here"
export EDISTRIBUCION_PASSWORD="password here"
```

Run the client without arguments:

```
./bin/contadores
<Your CUPS address will be here>
Potencia actual:  0.2
Potencia contratada:  5.75
Porcentage:  3,48%
Estado ICP:  Abierto
Totalizador:  14.811
```

## Database store

There's a sample client that stores received metrics in a sqlite database.

```
export EDISTRIBUCION_USERNAME="username here"
export EDISTRIBUCION_PASSWORD="password here"
```

```
make
./bin/edistribucion-store -db edistribucion.sqlite
```

## Related

* https://github.com/azogue/aiopvpc
* https://github.com/trocotronic/edistribucion
* https://github.com/uvejota/edistribucion
* https://github.com/uvejota/homeassistant-edata
