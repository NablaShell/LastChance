// File art generator.tsx
import React, { useMemo } from 'react';

interface FileArtProps {
  fileHash: string;
  fileName: string;
  size?: number;
  onClick?: () => void;
}

// MurmurHash-like simple deterministic generator
const seededRandom = (seed: number) => {
  return () => {
    seed = (seed * 9301 + 49297) % 233280;
    return seed / 233280;
  };
};

const FileArt: React.FC<FileArtProps> = ({ fileHash, fileName, size = 200, onClick }) => {
  const artSvg = useMemo(() => {
    // Convert the first 8 characters of the hash to a number
    const hashNum = parseInt(fileHash.slice(0, 8), 16);
    const rand = seededRandom(hashNum);
    
    const ext = fileName.split('.').pop()?.toLowerCase() || '';
    
    // Determining the file type
    const fileType = 
      ext.match(/jpg|jpeg|png|gif|webp|bmp|svg/) ? 'image' :
      ext.match(/mp4|webm|mov|avi|mkv|flv/) ? 'video' :
      ext.match(/mp3|wav|ogg|flac|m4a|aac/) ? 'audio' :
      ext.match(/pdf|doc|docx|txt|md|rtf/) ? 'document' :
      ext.match(/zip|rar|7z|tar|gz|bz2/) ? 'archive' :
      ext.match(/exe|msi|deb|rpm|appimage/) ? 'executable' :
      ext.match(/iso|img|dmg/) ? 'disk' : 'default';
    
    // Generate based on type (pass ext only where needed)
    return generateArt(hashNum, fileType, ext, rand);
  }, [fileHash, fileName]);

  return (
    <div 
      className="file-art-wrapper"
      style={{ 
        width: size, 
        height: size, 
        cursor: onClick ? 'pointer' : 'default',
        borderRadius: '8px',
        overflow: 'hidden',
        transition: 'transform 0.2s',
      }}
      onClick={onClick}
    >
      <div 
        dangerouslySetInnerHTML={{ __html: artSvg }}
        style={{ width: '100%', height: '100%' }}
      />
    </div>
  );
};

const generateArt = (seed: number, type: string, ext: string, rand: () => number): string => {
  const generators: Record<string, (s: number, r: () => number, e: string) => string> = {
    image: generateGeometricArt,
    video: generateWaveformArt,
    audio: generateSoundwaveArt,
    document: generateDocumentArt,
    archive: generateHexagonArt,
    executable: generateCircuitArt,
    disk: generateRadialArt,
    default: generateCyberpunkArt,
  };
  
  const generator = generators[type] || generators.default;
  return generator(seed, rand, ext);
};

// Generators of different styles

const generateGeometricArt = (seed: number, rand: () => number, ext: string): string => {
  const colors = [
    `hsl(${seed % 360}, 70%, 60%)`,
    `hsl(${(seed * 2) % 360}, 80%, 50%)`,
    `hsl(${(seed * 3) % 360}, 60%, 70%)`,
  ];
  
  const shapes = [];
  for (let i = 0; i < 5; i++) {
    const x = rand() * 200;
    const y = rand() * 200;
    const r = 20 + rand() * 40;
    const color = colors[Math.floor(rand() * colors.length)];
    const rotation = rand() * 360;
    
    if (rand() > 0.5) {
      shapes.push(`<circle cx="${x}" cy="${y}" r="${r}" fill="${color}" opacity="0.3" transform="rotate(${rotation} ${x} ${y})" />`);
    } else {
      const points = [
        `${x},${y - r}`,
        `${x + r * 0.8},${y + r * 0.5}`,
        `${x - r * 0.8},${y + r * 0.5}`,
      ].join(' ');
      shapes.push(`<polygon points="${points}" fill="${color}" opacity="0.3" transform="rotate(${rotation} ${x} ${y})" />`);
    }
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0d1117" />
      <defs>
        <linearGradient id="g${seed}" x1="0" y1="0" x2="200" y2="200">
          <stop offset="0%" stop-color="${colors[0]}" stop-opacity="0.2" />
          <stop offset="100%" stop-color="${colors[1]}" stop-opacity="0.1" />
        </linearGradient>
      </defs>
      <rect width="200" height="200" fill="url(#g${seed})" />
      ${shapes.join('')}
      <text x="100" y="180" text-anchor="middle" fill="${colors[0]}" 
            font-family="monospace" font-size="11" opacity="0.6">.${ext}</text>
    </svg>
  `;
};

const generateWaveformArt = (seed: number, rand: () => number, ext: string): string => {
  const color = `hsl(${seed % 360}, 80%, 55%)`;
  const points: string[] = [];
  
  for (let i = 0; i <= 20; i++) {
    const x = i * 10;
    const y = 100 + Math.sin(i * 0.8 + seed) * 40 + rand() * 30;
    points.push(`${x},${y}`);
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0a0a0f" />
      <polyline points="${points.join(' ')}" fill="none" stroke="${color}" 
                stroke-width="2" opacity="0.8" />
      <polyline points="${points.join(' ')}" fill="${color}" opacity="0.1" 
                transform="translate(0, -2)" />
      <text x="100" y="180" text-anchor="middle" fill="${color}" 
            font-family="monospace" font-size="11" opacity="0.6">VIDEO</text>
    </svg>
  `;
};

const generateSoundwaveArt = (seed: number, rand: () => number, ext: string): string => {
  const color = `hsl(${seed % 360}, 75%, 50%)`;
  const bars: string[] = [];
  
  for (let i = 0; i < 12; i++) {
    const x = 15 + i * 15;
    const height = 20 + rand() * 120;
    const y = (200 - height) / 2;
    bars.push(`<rect x="${x}" y="${y}" width="8" height="${height}" rx="2" 
              fill="${color}" opacity="${0.3 + rand() * 0.7}" />`);
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0d1117" />
      ${bars.join('')}
      <text x="100" y="180" text-anchor="middle" fill="${color}" 
            font-family="monospace" font-size="11" opacity="0.6">AUDIO</text>
    </svg>
  `;
};

const generateDocumentArt = (seed: number, rand: () => number, ext: string): string => {
  const color = `hsl(${seed % 360}, 70%, 55%)`;
  const lines: string[] = [];
  
  for (let i = 0; i < 8; i++) {
    const y = 50 + i * 15;
    const width = 60 + rand() * 80;
    lines.push(`<rect x="30" y="${y}" width="${width}" height="4" rx="2" 
                fill="${color}" opacity="${0.2 + rand() * 0.5}" />`);
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0d1117" />
      <rect x="20" y="20" width="160" height="160" rx="4" fill="none" 
            stroke="${color}" stroke-width="1" opacity="0.3" />
      ${lines.join('')}
      <text x="100" y="180" text-anchor="middle" fill="${color}" 
            font-family="monospace" font-size="11" opacity="0.6">.${ext}</text>
    </svg>
  `;
};

const generateHexagonArt = (seed: number, rand: () => number, ext: string): string => {
  const colors = [
    `hsl(${seed % 360}, 70%, 55%)`,
    `hsl(${(seed + 120) % 360}, 70%, 55%)`,
  ];
  
  const hexagons: string[] = [];
  const positions = [
    [100, 80], [60, 110], [140, 110], [100, 140]
  ];
  
  positions.forEach(([cx, cy], i) => {
    const points: string[] = [];
    for (let j = 0; j < 6; j++) {
      const angle = (j * 60 + 30) * Math.PI / 180;
      const x = cx + 25 * Math.cos(angle);
      const y = cy + 25 * Math.sin(angle);
      points.push(`${x},${y}`);
    }
    hexagons.push(`<polygon points="${points.join(' ')}" fill="none" 
                   stroke="${colors[i % 2]}" stroke-width="1.5" opacity="0.4" />`);
  });
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0a0a0f" />
      ${hexagons.join('')}
      <text x="100" y="180" text-anchor="middle" fill="${colors[0]}" 
            font-family="monospace" font-size="11" opacity="0.6">ARCHIVE</text>
    </svg>
  `;
};

const generateCircuitArt = (seed: number, rand: () => number, ext: string): string => {
  const color = `hsl(${seed % 360}, 80%, 55%)`;
  const paths: string[] = [];
  
  for (let i = 0; i < 6; i++) {
    const x1 = rand() * 200;
    const y1 = rand() * 200;
    const x2 = rand() * 200;
    const y2 = rand() * 200;
    const midX = (x1 + x2) / 2;
    
    paths.push(`<path d="M${x1},${y1} L${midX},${y1} L${midX},${y2} L${x2},${y2}" 
                fill="none" stroke="${color}" stroke-width="1" opacity="0.3" />`);
    paths.push(`<circle cx="${x1}" cy="${y1}" r="2" fill="${color}" opacity="0.5" />`);
    paths.push(`<circle cx="${x2}" cy="${y2}" r="2" fill="${color}" opacity="0.5" />`);
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0d1117" />
      ${paths.join('')}
      <text x="100" y="180" text-anchor="middle" fill="${color}" 
            font-family="monospace" font-size="11" opacity="0.6">.${ext.slice(0, 4)}</text>
    </svg>
  `;
};

const generateRadialArt = (seed: number, rand: () => number, ext: string): string => {
  const colors = [
    `hsl(${seed % 360}, 70%, 60%)`,
    `hsl(${(seed + 180) % 360}, 70%, 60%)`,
  ];
  
  const rings: string[] = [];
  for (let i = 1; i <= 4; i++) {
    rings.push(`<circle cx="100" cy="100" r="${i * 25}" fill="none" 
                stroke="${colors[i % 2]}" stroke-width="1" opacity="0.2" 
                stroke-dasharray="${10 + rand() * 20}, ${5 + rand() * 10}" />`);
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0a0a0f" />
      ${rings.join('')}
      <circle cx="100" cy="100" r="10" fill="${colors[0]}" opacity="0.4" />
      <text x="100" y="180" text-anchor="middle" fill="${colors[0]}" 
            font-family="monospace" font-size="11" opacity="0.6">DISK</text>
    </svg>
  `;
};

const generateCyberpunkArt = (seed: number, rand: () => number, ext: string): string => {
  const color = `hsl(${seed % 360}, 80%, 55%)`;
  const elements: string[] = [];
  
  // Network
  elements.push(`<pattern id="grid${seed}" width="20" height="20" patternUnits="userSpaceOnUse">
    <path d="M 20 0 L 0 0 0 20" fill="none" stroke="${color}" stroke-width="0.5" opacity="0.1" />
  </pattern>`);
  elements.push(`<rect width="200" height="200" fill="url(#grid${seed})" />`);
  
  // Diagonals
  elements.push(`<line x1="0" y1="0" x2="200" y2="200" stroke="${color}" stroke-width="1" opacity="0.15" />`);
  elements.push(`<line x1="200" y1="0" x2="0" y2="200" stroke="${color}" stroke-width="1" opacity="0.1" />`);
  
  // Points
  for (let i = 0; i < 8; i++) {
    elements.push(`<circle cx="${rand() * 200}" cy="${rand() * 200}" r="${1 + rand() * 3}" 
                   fill="${color}" opacity="${0.3 + rand() * 0.5}" />`);
  }
  
  return `
    <svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg">
      <rect width="200" height="200" fill="#0d1117" />
      <defs>${elements[0]}</defs>
      ${elements.slice(1).join('')}
      <text x="100" y="180" text-anchor="middle" fill="${color}" 
            font-family="monospace" font-size="11" opacity="0.6">FILE</text>
    </svg>
  `;
};

export default FileArt;