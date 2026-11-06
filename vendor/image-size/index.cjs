const fs = require('node:fs');
const parser = require('safe-image-size');
const { imageSizeFromFile, setConcurrency } = require('safe-image-size/fromFile');

let disabledFS = false;

function imageSize(input, callback) {
  if (input instanceof Uint8Array) return parser.imageSize(input);
  if (typeof input !== 'string' || disabledFS) {
    throw new TypeError('invalid invocation. input should be a Uint8Array');
  }
  if (typeof callback === 'function') {
    imageSizeFromFile(input).then(
      result => process.nextTick(callback, null, result),
      error => process.nextTick(callback, error),
    );
    return;
  }

  // Preserve v1's bounded synchronous file API used by Metro's asset loader.
  const descriptor = fs.openSync(input, 'r');
  try {
    const { size } = fs.fstatSync(descriptor);
    if (size <= 0) throw new Error('Empty file');
    const buffer = new Uint8Array(Math.min(size, 512 * 1024));
    const bytesRead = fs.readSync(descriptor, buffer, 0, buffer.length, 0);
    return parser.imageSize(buffer.subarray(0, bytesRead));
  } finally {
    fs.closeSync(descriptor);
  }
}

module.exports = Object.assign(imageSize, {
  imageSize,
  default: imageSize,
  disableFS: value => { disabledFS = value; },
  disableTypes: parser.disableTypes,
  setConcurrency,
  types: parser.types,
});
