function unique(prefix) {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1000)}`
}

function waitForOverview(predicate, attempts = 60) {
  return cy.request('/api/overview').then(({ body }) => {
    if (predicate(body)) {
      return body
    }

    if (attempts <= 1) {
      throw new Error('Timed out waiting for overview state to match expected condition')
    }

    return cy.wait(2000).then(() => waitForOverview(predicate, attempts - 1))
  })
}

describe('Hive UI mobile-first management', () => {
  it('creates projects, drones, and links them', () => {
    const projectName = unique('project')
    const requirementTitle = `Requirement ${Date.now()}`
    const taskName = unique('task')
    const reviewName = unique('review')
    const droneName = unique('drone')
    const linkName = unique('link')

    cy.visit('/')
    cy.contains('Vinculum').should('be.visible')

    cy.get('[data-testid="project-name-input"]').clear().type(projectName)
    cy.intercept('POST', '/api/repositories').as('createProject')
    cy.get('[data-testid="create-project-button"]').click()
    cy.wait('@createProject').its('response.statusCode').should('eq', 200)
    cy.contains('Project created.').should('be.visible')
    waitForOverview((body) => body.repositories.some((item) => item.metadata?.name === projectName && item.status?.phase === 'Ready'))

    cy.contains('Requirements').click()
    cy.get('[data-testid="requirement-project-select"]').click()
    cy.contains('li', projectName).click()
    cy.get('[data-testid="requirement-title-input"]').clear().type(requirementTitle)
    cy.get('[data-testid="requirement-body-input"]').clear().type('# Context{enter}{enter}Create a tracked requirement.{enter}{enter}# Acceptance Criteria{enter}{enter}- It exists in the repository.')
    cy.intercept('POST', '/api/requirements').as('createRequirement')
    cy.get('[data-testid="create-requirement-button"]').click()
    cy.wait('@createRequirement').its('response.statusCode').should('eq', 200)
    cy.contains('Requirement created.').should('be.visible')
    cy.request('/api/overview').its('body.requirements').should((items) => {
      expect(items.some((item) => item.spec?.repositoryRef === projectName)).to.eq(true)
    })
    cy.request('/api/overview').its('body.requirements').then((items) => {
      const requirement = items.find((item) => item.spec?.repositoryRef === projectName)
      expect(requirement, 'created requirement').to.exist
      cy.wrap(requirement.status?.observedTitle || requirement.metadata?.name).as('requirementLabel')
    })

    cy.contains('Tasks').click()
    cy.get('[data-testid="task-project-select"]').click()
    cy.contains('li', projectName).click()
    cy.get('[data-testid="task-requirement-select"]').click()
    cy.get('@requirementLabel').then((requirementLabel) => {
      cy.contains('li', requirementLabel).click()
    })
    cy.get('[data-testid="task-name-input"]').clear().type(taskName)
    cy.intercept('POST', '/api/tasks').as('createTask')
    cy.get('[data-testid="create-task-button"]').click()
    cy.wait('@createTask').its('response.statusCode').should('eq', 200)
    cy.contains('Task created.').should('be.visible')
    waitForOverview((body) => body.tasks.some((item) => item.metadata?.name === taskName))

    cy.contains('Reviews').click()
    cy.get('[data-testid="review-project-select"]').click()
    cy.contains('li', projectName).click()
    cy.get('[data-testid="review-task-select"]').click()
    cy.contains('li', taskName).click()
    cy.get('[data-testid="review-name-input"]').clear().type(reviewName)
    cy.get('[data-testid="review-summary-input"]').clear().type('Manual review created from the smoke test.')
    cy.intercept('POST', '/api/reviews').as('createReview')
    cy.get('[data-testid="create-review-button"]').click()
    cy.wait('@createReview').its('response.statusCode').should('eq', 200)
    cy.contains('Review created.').should('be.visible')
    waitForOverview((body) => body.reviews.some((item) => item.metadata?.name === reviewName))

    cy.get('[data-testid="nav-row"]').should('exist')
    cy.contains('Drones').click()
    cy.get('[data-testid="drone-name-input"]').clear().type(droneName)
    cy.get('[data-testid="drone-username-input"]').clear().type(`${droneName}-bot`)
    cy.intercept('POST', '/api/drones').as('createDrone')
    cy.get('[data-testid="create-drone-button"]').click()
    cy.wait('@createDrone').its('response.statusCode').should('eq', 200)
    cy.contains('Drone created.').should('be.visible')
    waitForOverview((body) => body.drones.some((item) => item.metadata?.name === droneName && item.status?.forgejoReady === true))

    cy.contains('Projects').click()
    cy.get(`[data-testid="project-link-${projectName}"]`).click()
    cy.contains('Create link').should('be.visible')

    cy.contains('Drones').click()
    cy.get(`[data-testid="drone-link-${droneName}"]`).click()
    cy.contains('Create link').should('be.visible')

    cy.contains('Links').click()
    cy.get('[data-testid="link-name-input"]').clear().type(linkName)
    cy.intercept('POST', '/api/accesses').as('createLink')
    cy.get('[data-testid="create-link-button"]').click()
    cy.wait('@createLink').its('response.statusCode').should('eq', 200)
    cy.contains('Link created.').should('be.visible')
    waitForOverview((body) => body.accesses.some((item) => item.metadata?.name === linkName && item.status?.phase === 'Ready'))

    cy.request('/api/overview').then(({ body }) => {
      expect(body.repositories.some((item) => item.metadata?.name === projectName)).to.eq(true)
      expect(body.requirements.some((item) => item.spec?.repositoryRef === projectName)).to.eq(true)
      expect(body.tasks.some((item) => item.metadata?.name === taskName)).to.eq(true)
      expect(body.reviews.some((item) => item.metadata?.name === reviewName)).to.eq(true)
      expect(body.drones.some((item) => item.metadata?.name === droneName)).to.eq(true)
      expect(body.accesses.some((item) => item.metadata?.name === linkName)).to.eq(true)
    })
  })
})
