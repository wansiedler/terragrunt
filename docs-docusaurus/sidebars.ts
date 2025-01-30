import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Creating a sidebar enables you to:
 - create an ordered group of docs
 - render a sidebar for each doc of that group
 - provide next/previous navigation

 The sidebars can be generated from the filesystem, or explicitly defined here.

 Create as many sidebars as you want.
 */
const sidebars: SidebarsConfig = {
  // By default, Docusaurus generates a sidebar from the docs folder structure
  gettingStartedSidebar: [{ type: 'autogenerated', dirName: 'getting-started' }],
  featuresSidebar: [{ type: 'autogenerated', dirName: 'features' }],
  referenceSidebar: [{ type: 'autogenerated', dirName: 'reference' }],
  communitySidebar: [{ type: 'autogenerated', dirName: 'community' }],
  troubleshootingSidebar: [{ type: 'autogenerated', dirName: 'troubleshooting' }],
  migrationGuidesSidebar: [{ type: 'autogenerated', dirName: 'migration-guides' }],
};

export default sidebars;
