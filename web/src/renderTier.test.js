import { describe, it, expect } from 'vitest';
import { detectRenderTier } from './renderTier';

describe('detectRenderTier', () => {
  it('программные рендеры → software', () => {
    expect(detectRenderTier('ANGLE (Google, Vulkan SwiftShader Device (0x0000) zzz)')).toBe('software');
    expect(detectRenderTier('Mesa OffScreen llvmpipe (LLVM 17)')).toBe('software');
    expect(detectRenderTier('Software Webgl')).toBe('software');
    expect(detectRenderTier('ANGLE Basic Render (Software)')).toBe('software');
    expect(detectRenderTier('swiftshader')).toBe('software');
  });

  it('аппаратные рендеры → hardware', () => {
    expect(detectRenderTier('ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0)')).toBe('hardware');
    expect(detectRenderTier('ANGLE (Intel, Intel(R) UHD Graphics (0x00009A49) Direct3D11)')).toBe('hardware');
    expect(detectRenderTier('ANGLE (Google, Vulkan NVIDIA GeForce RTX 3060)')).toBe('hardware');
    expect(detectRenderTier('AMD Radeon 780M')).toBe('hardware');
  });

  it('пустая/недоступная строка → software (безопасный дефолт)', () => {
    expect(detectRenderTier('')).toBe('software');
    expect(detectRenderTier(null)).toBe('software');
    expect(detectRenderTier(undefined)).toBe('software');
  });

  it('Microsoft Basic Render Driver (WARP) → software', () => {
    expect(detectRenderTier('ANGLE (Microsoft, Basic Render Driver Direct3D11)')).toBe('software');
  });
});
