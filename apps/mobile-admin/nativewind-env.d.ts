/// <reference types="nativewind/types" />

// NativeWind 4.2 / RN 0.86 no longer declares a side-effect module for
// `*.css`, so `import '../global.css'` reports TS2882. Declare it here.
declare module '*.css' {}
