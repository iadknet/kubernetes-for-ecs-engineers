// The drill view's chat client: fetch + ReadableStream, parsing SSE by hand.
//
// Not htmx's SSE extension and not EventSource: the extension is a CDN
// dependency this app does not take, and EventSource cannot POST a body.
(function () {
  'use strict';

  var form = document.getElementById('chat-form');
  if (!form) return; // chat is off; nothing was rendered

  var log = document.getElementById('chat-log');
  var input = document.getElementById('chat-input');
  var sendButton = document.getElementById('chat-send');
  var model = document.getElementById('chat-model');
  var effort = document.getElementById('chat-effort');
  var resetButton = document.getElementById('chat-reset');

  function bubble(kind, text) {
    var el = document.createElement('div');
    el.className = 'chat-msg ' + kind;
    el.textContent = text;
    log.appendChild(el);
    log.scrollTop = log.scrollHeight;
    return el;
  }

  // The card is swapped in place by htmx, so the id has to be read at send
  // time rather than captured when the page loaded.
  function currentCard() {
    var card = document.getElementById('card');
    return card ? card.dataset.card || '' : '';
  }

  // The answer streams as plain text and is replaced, once, by the server's
  // rendered markdown when the turn completes. The `md` class is what stops
  // .chat-msg's pre-wrap from showing every newline goldmark left inside a <p>.
  function render(el, html) {
    if (!html) return;

    el.innerHTML = html;
    el.classList.add('md');

    log.scrollTop = log.scrollHeight;

    // A diagram arrives as <pre class="mermaid"> and nothing else draws it: the
    // page's htmx:afterSwap hook (layout.html) never fires for chat, which is
    // not htmx.
    var diagrams = el.querySelectorAll('pre.mermaid:not([data-processed])');
    if (!window.mermaid || !diagrams.length) return;

    // Deck diagrams are gated by TestDiagramStyle before they ship; one a model
    // improvised mid-answer is not, so a diagram that will not parse has to
    // degrade well. Two options are needed and neither is enough alone:
    // suppressErrors stops run() throwing and abandoning later nodes, and
    // suppressErrorRendering (layout.html) stops the error graphic. Together
    // they leave the node emptied and drawn as nothing — a silent gap — so the
    // source is stashed first and put back on anything that did not render.
    var sources = Array.prototype.map.call(diagrams, function (node) {
      return node.textContent;
    });

    window.mermaid.run({ nodes: diagrams, suppressErrors: true }).finally(function () {
      Array.prototype.forEach.call(diagrams, function (node, i) {
        if (node.querySelector('svg')) return;

        node.textContent = sources[i];
        node.dataset.failed = 'true';
      });

      log.scrollTop = log.scrollHeight;
    });
  }

  // One frame is "event: <name>\ndata: <json>". The payload is JSON so an
  // answer containing blank lines cannot terminate its own event.
  function handleFrame(frame, answer) {
    var name = '';
    var data = '';

    frame.split('\n').forEach(function (line) {
      if (line.indexOf('event: ') === 0) name = line.slice(7);
      else if (line.indexOf('data: ') === 0) data = line.slice(6);
    });

    if (!data) return;

    if (name === 'delta') {
      answer.textContent += JSON.parse(data);
      log.scrollTop = log.scrollHeight;
    } else if (name === 'done') {
      render(answer, JSON.parse(data));
    } else if (name === 'error') {
      // Deliberately no render: a turn that died partway has partial markdown,
      // and the plain text it already streamed is what actually arrived.
      answer.className = 'chat-msg err';
      answer.textContent += (answer.textContent ? '\n\n' : '') + JSON.parse(data);
    }
  }

  async function ask(question, answer) {
    var res = await fetch('/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        card: currentCard(),
        message: question,
        model: model ? model.value : '',
        effort: effort ? effort.value : ''
      })
    });

    if (!res.ok) {
      answer.className = 'chat-msg err';
      answer.textContent = (await res.text()).trim() || 'request failed: ' + res.status;
      return;
    }

    var reader = res.body.getReader();
    var decoder = new TextDecoder();
    var buffer = '';

    for (;;) {
      var chunk = await reader.read();
      if (chunk.done) break;

      buffer += decoder.decode(chunk.value, { stream: true });

      var split;
      while ((split = buffer.indexOf('\n\n')) >= 0) {
        handleFrame(buffer.slice(0, split), answer);
        buffer = buffer.slice(split + 2);
      }
    }
  }

  form.addEventListener('submit', async function (e) {
    e.preventDefault();

    var question = input.value.trim();
    if (!question) return;

    if (!currentCard()) {
      bubble('err', 'There is no card on screen to ask about.');
      return;
    }

    input.value = '';
    sendButton.disabled = true;
    bubble('you', question);

    var answer = bubble('claude', '');

    try {
      await ask(question, answer);
    } catch (err) {
      answer.className = 'chat-msg err';
      answer.textContent = String(err);
    } finally {
      sendButton.disabled = false;
      input.focus();
    }
  });

  resetButton.addEventListener('click', async function () {
    resetButton.disabled = true;

    try {
      // The content type is not decoration: the server requires it, because a
      // request a hostile page could forge cannot carry it.
      await fetch('/chat/reset', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
      });
      log.replaceChildren();
      bubble('note', 'Started a new conversation.');
    } finally {
      resetButton.disabled = false;
    }
  });

  // Enter asks; shift+enter is a newline. The page's own 1-4 grading shortcuts
  // already ignore keys typed inside a textarea.
  input.addEventListener('keydown', function (e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      form.requestSubmit();
    }
  });
})();
