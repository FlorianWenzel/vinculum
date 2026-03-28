import { _ as __nuxt_component_0 } from './nuxt-link-obH4GEin.mjs';
import { defineComponent, mergeProps, withCtx, createTextVNode, useSSRContext } from 'file:///Users/florian/projects/vinculum/apps/site/node_modules/vue/index.mjs';
import { ssrRenderAttrs, ssrRenderList, ssrRenderAttr, ssrInterpolate, ssrRenderComponent } from 'file:///Users/florian/projects/vinculum/apps/site/node_modules/vue/server-renderer/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/ufo/dist/index.mjs';
import './server.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/ofetch/dist/node.mjs';
import '../_/renderer.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/vue-bundle-renderer/dist/runtime.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/h3/dist/index.mjs';
import '../nitro/nitro.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/destr/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/hookable/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/node-mock-http/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unstorage/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unstorage/drivers/fs.mjs';
import 'node:crypto';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unstorage/drivers/fs-lite.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unstorage/drivers/lru-cache.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/ohash/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/klona/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/defu/dist/defu.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/scule/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unctx/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/radix3/dist/index.mjs';
import 'node:fs';
import 'node:url';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/pathe/dist/index.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unhead/dist/server.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/devalue/index.js';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unhead/dist/utils.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/unhead/dist/plugins.mjs';
import 'file:///Users/florian/projects/vinculum/apps/site/node_modules/vue-router/vue-router.node.mjs';

const _sfc_main = /* @__PURE__ */ defineComponent({
  __name: "index",
  __ssrInlineRender: true,
  setup(__props) {
    const pillars = [
      {
        title: "Autonomous Delivery Loops",
        text: "Coder, reviewer, tester, and deployer drones work through requirements, implementation, review, and release as durable Kubernetes workloads."
      },
      {
        title: "Cluster-Native Control Plane",
        text: "The orchestrator owns CRDs, schedules drone jobs, tracks task state, and exposes an API and Hive UI for operational visibility."
      },
      {
        title: "Built-In Platform Stack",
        text: "Forgejo, Keycloak, PostgreSQL, bootstrap automation, and Helm packaging ship together so a clean cluster can become a working development platform quickly."
      }
    ];
    const surfaces = [
      { label: "Hive UI", href: "https://app.vincula.dev", note: "Control surface for drones, repositories, tasks, and reviews." },
      { label: "Vinculum Code", href: "https://git.vincula.dev", note: "Forgejo-based code forge with OIDC and project automation." },
      { label: "Identity", href: "https://id.vincula.dev", note: "Keycloak realm used for browser auth and service integration." }
    ];
    const docs = [
      { title: "Deployment Model", path: "/docs#deployment", text: "Publishing flow, GHCR images, OCI Helm charts, and GitHub Actions behavior." },
      { title: "Operational Reset", path: "/docs#operations", text: "Cluster reset, reinstall expectations, ingress, and Cloudflare tunnel placement." },
      { title: "Architecture Guide", path: "/docs#architecture", text: "How the operator, infrastructure bootstrap, drone runtime, and UI fit together." }
    ];
    return (_ctx, _push, _parent, _attrs) => {
      const _component_NuxtLink = __nuxt_component_0;
      _push(`<div${ssrRenderAttrs(mergeProps({ class: "site-shell" }, _attrs))}><section class="hero"><div class="hero__nav"><div><p class="eyebrow">Vinculum</p><p class="hero__tag">Autonomous software delivery on Kubernetes</p></div><nav><a href="/docs">Docs</a><a href="https://github.com/florianwenzel/vinculum">Source</a></nav></div><div class="hero__content"><div class="hero__copy"><p class="kicker">Collective orchestration for real repositories</p><h1>Turn requirements into reviewed, tested, and deployed software.</h1><p class="lead"> Vinculum combines drone workers, a Kubernetes operator, built-in identity, a Forgejo control surface, and deployable Helm charts into one cluster-native platform. </p><div class="hero__actions"><a class="button button--primary" href="/docs">Read the docs</a><a class="button button--ghost" href="https://git.vincula.dev">Open Vinculum Code</a></div></div><div class="hero__panel"><span class="panel__title">Runtime surfaces</span><ul><!--[-->`);
      ssrRenderList(surfaces, (surface) => {
        _push(`<li><a${ssrRenderAttr("href", surface.href)}>${ssrInterpolate(surface.label)}</a><span>${ssrInterpolate(surface.note)}</span></li>`);
      });
      _push(`<!--]--></ul></div></div></section><section class="section section--tight"><div class="section__header"><p class="eyebrow">Platform</p><h2>Purpose-built for agent swarms, not single-shot demos.</h2></div><div class="card-grid card-grid--three"><!--[-->`);
      ssrRenderList(pillars, (pillar) => {
        _push(`<article class="card"><h3>${ssrInterpolate(pillar.title)}</h3><p>${ssrInterpolate(pillar.text)}</p></article>`);
      });
      _push(`<!--]--></div></section><section class="section"><div class="section__header"><p class="eyebrow">Workflow</p><h2>One platform, four persistent layers.</h2></div><div class="timeline"><div class="timeline__item"><strong>1. Requirements and planning</strong><p>Requirements become durable resources instead of chat history, so the operator can schedule, review, and revisit work later.</p></div><div class="timeline__item"><strong>2. Drone execution</strong><p>Workers run in isolated pods with mounted instructions, auth, and repository access scoped through Kubernetes objects.</p></div><div class="timeline__item"><strong>3. Platform bootstrap</strong><p>Vinculum Infra reconciles Keycloak and Forgejo so identity, organizations, and OIDC wiring come up with the stack.</p></div><div class="timeline__item"><strong>4. Ship and operate</strong><p>Helm charts, GHCR images, and cluster ingress make the stack installable, reproducible, and externally reachable.</p></div></div></section><section class="section section--tight"><div class="section__header"><p class="eyebrow">Docs</p><h2>Start with the operational model.</h2></div><div class="card-grid"><!--[-->`);
      ssrRenderList(docs, (item) => {
        _push(`<article class="card card--link"><h3>${ssrInterpolate(item.title)}</h3><p>${ssrInterpolate(item.text)}</p>`);
        _push(ssrRenderComponent(_component_NuxtLink, {
          to: item.path
        }, {
          default: withCtx((_, _push2, _parent2, _scopeId) => {
            if (_push2) {
              _push2(`Open section`);
            } else {
              return [
                createTextVNode("Open section")
              ];
            }
          }),
          _: 2
        }, _parent));
        _push(`</article>`);
      });
      _push(`<!--]--></div></section></div>`);
    };
  }
});
const _sfc_setup = _sfc_main.setup;
_sfc_main.setup = (props, ctx) => {
  const ssrContext = useSSRContext();
  (ssrContext.modules || (ssrContext.modules = /* @__PURE__ */ new Set())).add("pages/index.vue");
  return _sfc_setup ? _sfc_setup(props, ctx) : void 0;
};

export { _sfc_main as default };
//# sourceMappingURL=index-DA7XwErw.mjs.map
