
import * as fs from 'fs-extra';
import * as path from 'path';
import { glob } from 'glob';

interface OrpheusManifest {
  id: string;
  title: string;
  description: string;
  author: string;
  tags: string[];
  runtime: string;
  category: string;
  version: string;
  files?: string[];
}

const EXAMPLES_DIR = path.join(__dirname, '../examples');
const DOCS_DIR = path.join(__dirname, '../documentation/examples');
const MINT_JSON_PATH = path.join(__dirname, '../documentation/mint.json');

async function main() {
  console.log('🔄 Syncing example agents documentation...');

  // 1. Find all orpheus.json files
  const manifestFiles = await glob('**/orpheus.json', { cwd: EXAMPLES_DIR });
  
  if (manifestFiles.length === 0) {
    console.log('⚠️  No example agents found (no orpheus.json files).');
    return;
  }

  console.log(`Found ${manifestFiles.length} example agents.`);

  // 2. Process each manifest
  const categories: Record<string, string[]> = {};

  for (const manifestFile of manifestFiles) {
    const agentDir = path.dirname(path.join(EXAMPLES_DIR, manifestFile));
    const manifestPath = path.join(EXAMPLES_DIR, manifestFile);
    
    try {
      const manifest: OrpheusManifest = await fs.readJson(manifestPath);
      
      // Validate manifest
      if (!manifest.id || !manifest.title || !manifest.category) {
        console.error(`❌ Invalid manifest in ${manifestFile}: Missing required fields.`);
        continue;
      }

      console.log(`Processing ${manifest.id}...`);

      // Read Source Files
      const readmePath = path.join(agentDir, 'README.md');
      const agentYamlPath = path.join(agentDir, 'agent.yaml');
      // Detect runtime file
      let codeFile = 'agent.py';
      if (manifest.runtime.includes('node')) {
        codeFile = 'agent.js';
        if (!fs.existsSync(path.join(agentDir, codeFile))) {
             codeFile = 'index.ts'; // fallback check
        }
      }
      
      const readmeContent = fs.existsSync(readmePath) ? await fs.readFile(readmePath, 'utf-8') : 'No description provided.';
      const yamlContent = fs.existsSync(agentYamlPath) ? await fs.readFile(agentYamlPath, 'utf-8') : '# No agent.yaml found';
      
      // Determine which files to show in tabs
      const filesToShow: { name: string; content: string; language: string }[] = [];
      
      // Always show agent.yaml first
      filesToShow.push({ name: 'agent.yaml', content: yamlContent, language: 'yaml' });

      if (manifest.files && manifest.files.length > 0) {
        // Multi-file support from manifest
        for (const file of manifest.files) {
          const filePath = path.join(agentDir, file);
          if (fs.existsSync(filePath)) {
             const content = await fs.readFile(filePath, 'utf-8');
             // Simple extension detection
             const ext = path.extname(file);
             let lang = 'text';
             if (ext === '.py') lang = 'python';
             if (ext === '.js' || ext === '.ts') lang = 'javascript';
             if (ext === '.sh') lang = 'bash';
             if (ext === '.json') lang = 'json';
             
             filesToShow.push({ name: file, content, language: lang });
          }
        }
      } else {
        // Fallback: Auto-detect single file
        let codeFile = 'agent.py';
        if (manifest.runtime.includes('node')) {
            codeFile = 'agent.js';
            if (!fs.existsSync(path.join(agentDir, codeFile))) {
                codeFile = 'index.ts';
            }
        }
        const codePath = path.join(agentDir, codeFile);
        if (fs.existsSync(codePath)) {
            const content = await fs.readFile(codePath, 'utf-8');
            const lang = manifest.runtime.includes('python') ? 'python' : 'javascript';
            filesToShow.push({ name: codeFile, content, language: lang });
        }
      }

      // Generate Tabs MDX
      let tabsContent = '<Tabs>\n';
      for (const file of filesToShow) {
          // Redact API keys in content
          let safeContent = file.content;
          
          // Pattern for OpenAI keys: sk-... characters
          safeContent = safeContent.replace(/sk-[a-zA-Z0-9\-_]{20,}/g, 'sk-proj-**********************');
           
          // Pattern for generic keys in yaml/env vars (e.g. KEY=abc...)
          // Only safe if we are sure it's a key. Let's target specific vars or sensitive patterns if possible.
          // For now, let's stick to the high-entropy sk- keys which are the most common leak risk here.
          
          tabsContent += `  <Tab title="${file.name}">\n\`\`\`${file.language}\n${safeContent}\n\`\`\`\n  </Tab>\n`;
      }
      tabsContent += '</Tabs>';

      // Generate MDX Content
      const mdxContent = `---
title: '${manifest.title}'
description: '${manifest.description}'
---

${readmeContent}

## Source Code

${tabsContent}
`;

      // Write MDX file
      const categoryDir = path.join(DOCS_DIR, manifest.category.toLowerCase());
      await fs.ensureDir(categoryDir);
      const mdxPath = path.join(categoryDir, `${manifest.id}.mdx`);
      await fs.writeFile(mdxPath, mdxContent);

      // Track for navigation
      if (!categories[manifest.category]) {
        categories[manifest.category] = [];
      }
      categories[manifest.category].push(`examples/${manifest.category.toLowerCase()}/${manifest.id}`);

    } catch (err) {
      console.error(`❌ Error processing ${manifestFile}:`, err);
    }
  }

  // 3. Update mint.json Navigation
  await updateNavigation(categories);

  console.log('✅ Documentation sync complete!');
}

async function updateNavigation(newCategories: Record<string, string[]>) {
  const mintConfig = await fs.readJson(MINT_JSON_PATH);
  
  // Find "Examples" group or create it
  let examplesGroup = mintConfig.navigation.find((g: any) => g.group === 'Examples');
  if (!examplesGroup) {
      examplesGroup = { group: 'Examples', pages: [] };
      mintConfig.navigation.push(examplesGroup);
  }

  // Convert our category map to specific pages list
  // Note: Mintlify supports sub-groups in navigation? 
  // For simplicity, let's just flattened list or minimal grouping if supported.
  // Docs usually structure as "examples/basic/calculator" so we just provide the strings.
  
  const allExamplePages: string[] = [];
  
  // Sort categories to keep order stable
  const sortedCategories = Object.keys(newCategories).sort();
  
  for (const cat of sortedCategories) {
      // Sort pages within category
      const pages = newCategories[cat].sort();
      allExamplePages.push(...pages);
  }

  // Replace the pages in the Examples group
  examplesGroup.pages = allExamplePages;

  await fs.writeJson(MINT_JSON_PATH, mintConfig, { spaces: 2 });
}

main().catch(console.error);
