# cbr2cbz

Ported [original shell version](https://git.zaks.web.za/thisiszeev/cbr2cbz) to golang so it could be run on most oses.

Doesn't need zip or rar to be installed anywhere, just point at directory and go.

![screenshot](./docs/image.png)

## Usage

```
cbr2cbz convert ~/Comics
```

## Installing

You should be able to goto the [latest release](https://github.com/halkeye/cbr2cbz/releases/latest) and download whatever verison you need for your os.

If you can run python, I found [install-release](https://github.com/Rishang/install-release) is good at installing github based releases.

## Reasoning

Was playing around with metadata tools such as comictagger, and it can't natively handle writing cbr files. I believe from my googling because rar is licensed and you need to use thier tooling for writes, but not certain.  
Plus the original script didn't actually run properly on my machine.

## Credits

[original shell version](https://git.zaks.web.za/thisiszeev/cbr2cbz)
