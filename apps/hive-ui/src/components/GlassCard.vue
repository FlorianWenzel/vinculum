<template>
  <article class="glass-card glass">
    <header v-if="$slots.title" class="glass-card__header">
      <slot name="title" />
    </header>

    <div class="glass-card__body">
      <slot name="content">
        <slot />
      </slot>
    </div>
  </article>
</template>
