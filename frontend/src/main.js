import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';

const target = document.getElementById('app');
target.replaceChildren();

const app = mount(App, {
  target,
});

export default app;
